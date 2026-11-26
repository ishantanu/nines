package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

type cliDevice struct {
	FlowHash, Tenant, Challenge, UserCode, SessionHash, Status string
	Expires, NextPoll                                          time.Time
	Interval                                                   int
}

func (s *Service) cliCapabilities(w http.ResponseWriter, r *http.Request) {
	authJSON(w, 200, v1.CLIAuthenticationCapabilities{APIVersion: "nines.dev/v1alpha1", Kind: "CLIAuthenticationCapabilities", Browser: true, Device: true, APIKey: true, SessionMaxSeconds: 3600})
}
func (s *Service) cliDeviceStart(w http.ResponseWriter, r *http.Request) {
	if !s.rateLogin(w, r) {
		return
	}
	var in v1.CLIDeviceStart
	if err := decode(w, r, &in); err != nil {
		authFailure(w, err)
		return
	}
	if !s.knownTenant(in.Tenant) || !validOpaque(in.Challenge) {
		authFailure(w, ErrInvalid)
		return
	}
	var code [5]byte
	if _, err := rand.Read(code[:]); err != nil {
		authFailure(w, err)
		return
	}
	user := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(code[:])
	user = user[:4] + "-" + user[4:]
	ticket, secret := randomToken(), randomToken()
	expires := s.Now().Add(5 * time.Minute)
	f := cliFlow{Tenant: in.Tenant, Challenge: in.Challenge, Expires: expires, DeviceHash: hash(secret), UserCode: user}
	d := cliDevice{FlowHash: hash(ticket), Tenant: in.Tenant, Challenge: in.Challenge, UserCode: user, Status: "authorization_pending", Expires: expires, Interval: 5}
	err := s.repo.Update(r.Context(), "cli:requests", func(tx Tx) error {
		rows, e := tx.List("cli_flow", "global")
		if e != nil {
			return e
		}
		if len(rows) >= 1000 {
			return ErrConflict
		}
		if e = put(tx, "cli_flow", "global", hash(ticket), f, expires); e != nil {
			return e
		}
		return put(tx, "cli_device", "global", hash(secret), d, expires)
	})
	if err != nil {
		authFailure(w, err)
		return
	}
	authJSON(w, 200, v1.CLIDeviceAuthorization{DeviceCode: secret, UserCode: user, VerificationURI: s.config.PublicURL + "/auth/cli/authorize?request=" + ticket, ExpiresAt: expires, IntervalSeconds: 5})
}
func (s *Service) cliDevicePoll(w http.ResponseWriter, r *http.Request) {
	var in v1.CLIDevicePoll
	if err := decode(w, r, &in); err != nil {
		authFailure(w, err)
		return
	}
	if !validOpaque(in.DeviceCode) || !validOpaque(in.CodeVerifier) {
		authFailure(w, ErrInvalid)
		return
	}
	var d cliDevice
	err := s.repo.View(r.Context(), func(tx Tx) error {
		var e error
		d, e = read[cliDevice](tx, "cli_device", "global", hash(in.DeviceCode))
		return e
	})
	if errors.Is(err, ErrNotFound) {
		authJSON(w, 200, v1.CLIDeviceResult{Status: "expired_token"})
		return
	}
	if err != nil {
		authFailure(w, err)
		return
	}
	sum := sha256.Sum256([]byte(in.CodeVerifier))
	if !equal(d.Challenge, base64.RawURLEncoding.EncodeToString(sum[:])) {
		authFailure(w, ErrForbidden)
		return
	}
	result := v1.CLIDeviceResult{}
	token := "wp_cli_" + randomToken()
	var actor Actor
	err = s.repo.Update(r.Context(), "cli:flow:"+d.FlowHash, func(tx Tx) error {
		var e error
		d, e = read[cliDevice](tx, "cli_device", "global", hash(in.DeviceCode))
		if errors.Is(e, ErrNotFound) {
			result.Status = "expired_token"
			return nil
		}
		if e != nil {
			return e
		}
		if !d.Expires.After(s.Now()) {
			result.Status = "expired_token"
			return tx.Delete("cli_device", "global", hash(in.DeviceCode))
		}
		if strings.HasSuffix(r.URL.Path, "/cancel") {
			if e = tx.Delete("cli_flow", "global", d.FlowHash); e != nil {
				return e
			}
			result.Status = "cancelled"
			return tx.Delete("cli_device", "global", hash(in.DeviceCode))
		}
		if s.Now().Before(d.NextPoll) {
			d.Interval = min(30, d.Interval+5)
			d.NextPoll = s.Now().Add(time.Duration(d.Interval) * time.Second)
			result.Status = "slow_down"
			result.IntervalSeconds = d.Interval
			return put(tx, "cli_device", "global", hash(in.DeviceCode), d, d.Expires)
		}
		result.Status = d.Status
		switch d.Status {
		case "authorization_pending":
			d.NextPoll = s.Now().Add(time.Duration(d.Interval) * time.Second)
			result.IntervalSeconds = d.Interval
			return put(tx, "cli_device", "global", hash(in.DeviceCode), d, d.Expires)
		case "access_denied":
			return tx.Delete("cli_device", "global", hash(in.DeviceCode))
		case "approved":
			actor, e = s.sessionActorTx(tx, d.SessionHash)
			if e != nil && !errors.Is(e, ErrForbidden) && !errors.Is(e, ErrNotFound) {
				return e
			}
			if e != nil || actor.Grants[d.Tenant] == "" {
				result.Status = "access_denied"
				return tx.Delete("cli_device", "global", hash(in.DeviceCode))
			}
			expires := minTime(actor.Expires, s.Now().Add(time.Hour))
			grant := cliGrant{SessionHash: d.SessionHash, Tenant: d.Tenant, Expires: expires}
			if e = put(tx, "cli_token", "global", hash(token), grant, expires); e != nil {
				return e
			}
			if e = tx.Delete("cli_device", "global", hash(in.DeviceCode)); e != nil {
				return e
			}
			result = v1.CLIDeviceResult{Status: "authorized", Token: token, Tenant: d.Tenant, Identity: actor.ID, ExpiresAt: expires}
			return nil
		}
		return ErrForbidden
	})
	if err != nil {
		authFailure(w, err)
		return
	}
	if result.Status == "authorized" {
		if err = s.RecordAudit(r.Context(), d.Tenant, actor, "cli_sign_in", "/auth/cli/device/token", "succeeded", "", ""); err != nil {
			_ = s.revokeCLI(r.Context(), token)
			authFailure(w, err)
			return
		}
	}
	authJSON(w, 200, result)
}
