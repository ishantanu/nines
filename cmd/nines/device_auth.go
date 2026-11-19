package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
)

var deviceUserCode = regexp.MustCompile(`^[A-Z2-7]{4}-[A-Z2-7]{4}$`)

func authCapabilities(ctx context.Context, o *options) (v1.CLIAuthenticationCapabilities, error) {
	var capabilities v1.CLIAuthenticationCapabilities
	err := authRequest(ctx, o.httpClient(), o.Server, "", "", "GET", "/auth/cli/capabilities", nil, &capabilities)
	if ae, ok := err.(*client.APIError); ok && ae.StatusCode == 404 {
		return capabilities, &cliError{"unavailable", "Server does not advertise CLI login capabilities; use NINES_API_KEY or upgrade the server", 6}
	}
	if err == nil && (capabilities.APIVersion != "nines.dev/v1alpha1" || capabilities.Kind != "CLIAuthenticationCapabilities") {
		return capabilities, usage("invalid CLI authentication capability response")
	}
	return capabilities, err
}
func deviceLogin(ctx context.Context, o *options, out io.Writer) (credential, error) {
	var zero credential
	capabilities, err := authCapabilities(ctx, o)
	if err != nil {
		return zero, err
	}
	if !capabilities.Device {
		return zero, &cliError{"unavailable", "Device login is not enabled on this server; use NINES_API_KEY or configured browser login", 6}
	}
	verifier, err := nonce()
	if err != nil {
		return zero, err
	}
	sum := sha256.Sum256([]byte(verifier))
	var start v1.CLIDeviceAuthorization
	if err = authRequest(ctx, o.httpClient(), o.Server, "", "", "POST", "/auth/cli/device/start", v1.CLIDeviceStart{Tenant: o.Tenant, Challenge: base64.RawURLEncoding.EncodeToString(sum[:])}, &start); err != nil {
		return zero, err
	}
	base, _ := url.Parse(o.Server)
	u, e := url.Parse(start.VerificationURI)
	if e != nil || u.Scheme != base.Scheme || u.Host != base.Host || u.User != nil || u.Fragment != "" || u.Path != "/auth/cli/authorize" || len(u.Query()) != 1 || len(u.Query()["request"]) != 1 || len(u.Query().Get("request")) != 43 || len(start.DeviceCode) != 43 || !deviceUserCode.MatchString(start.UserCode) || !start.ExpiresAt.After(time.Now()) || start.ExpiresAt.After(time.Now().Add(6*time.Minute)) || start.IntervalSeconds < 1 || start.IntervalSeconds > 30 {
		return zero, usage("invalid device authorization response")
	}
	login, cancel := context.WithDeadline(ctx, start.ExpiresAt)
	defer cancel()
	complete := false
	defer func() {
		if !complete {
			cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			_ = authRequest(cleanup, o.httpClient(), o.Server, "", "", "POST", "/auth/cli/device/cancel", v1.CLIDevicePoll{DeviceCode: start.DeviceCode, CodeVerifier: verifier}, nil)
		}
	}()
	if _, err = fmt.Fprintf(out, "On a trusted browser, open:\n%s\nConfirm code %s for tenant %s at %s.\n", start.VerificationURI, start.UserCode, o.Tenant, o.Server); err != nil {
		return zero, err
	}
	interval := time.Duration(start.IntervalSeconds) * time.Second
	for {
		timer := time.NewTimer(interval)
		select {
		case <-login.Done():
			timer.Stop()
			return zero, &cliError{"unauthenticated", "Device sign-in expired or was cancelled; start login again", 3}
		case <-timer.C:
		}
		var result v1.CLIDeviceResult
		if err = authRequest(login, o.httpClient(), o.Server, "", "", "POST", "/auth/cli/device/token", v1.CLIDevicePoll{DeviceCode: start.DeviceCode, CodeVerifier: verifier}, &result); err != nil {
			return zero, err
		}
		switch result.Status {
		case "authorization_pending", "slow_down":
			if result.IntervalSeconds < 1 || result.IntervalSeconds > 30 {
				return zero, usage("invalid device polling interval")
			}
			interval = max(interval, time.Duration(result.IntervalSeconds)*time.Second)
		case "authorized":
			if len(result.Token) != 50 || result.Token[:7] != "wp_cli_" || result.Tenant != o.Tenant || result.Identity == "" || !result.ExpiresAt.After(time.Now()) || result.ExpiresAt.After(time.Now().Add(61*time.Minute)) {
				return zero, usage("invalid device credential response")
			}
			complete = true
			return credential{Server: o.Server, Tenant: o.Tenant, Token: result.Token, Expires: result.ExpiresAt, Identity: result.Identity, Method: "device"}, nil
		case "access_denied", "expired_token":
			return zero, &cliError{"unauthenticated", "Device authorization was denied, revoked or expired; start login again", 3}
		default:
			return zero, usage("invalid device authorization status")
		}
	}
}
