package identity

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func deviceCall(t *testing.T, s *Service, path string, body any, want int) v1.CLIDeviceResult {
	t.Helper()
	b, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", path, strings.NewReader(string(b)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s: HTTP %d, expected %d", path, w.Code, want)
	}
	var result v1.CLIDeviceResult
	if want == 200 && json.Unmarshal(w.Body.Bytes(), &result) != nil {
		t.Fatal("invalid device result")
	}
	return result
}
func startDevice(t *testing.T, s *Service) (v1.CLIDeviceAuthorization, string) {
	t.Helper()
	verifier := randomToken()
	sum := sha256.Sum256([]byte(verifier))
	body, _ := json.Marshal(v1.CLIDeviceStart{Tenant: "alpha", Challenge: base64.RawURLEncoding.EncodeToString(sum[:])})
	r := httptest.NewRequest("POST", "/auth/cli/device/start", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("start device HTTP %d", w.Code)
	}
	var out v1.CLIDeviceAuthorization
	if json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatal("bad authorization")
	}
	if strings.Contains(out.VerificationURI, out.DeviceCode) || len(out.UserCode) != 9 {
		t.Fatal("unsafe device response")
	}
	return out, verifier
}
func decideDevice(t *testing.T, s *Service, session string, d v1.CLIDeviceAuthorization, decision, code string, want int) {
	t.Helper()
	u, _ := url.Parse(d.VerificationURI)
	ticket := u.Query().Get("request")
	form := url.Values{"request": {ticket}, "csrf": {hash("csrf:" + session)}, "decision": {decision}, "userCode": {code}}
	r := httptest.NewRequest("POST", "/auth/cli/approve", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: s.cookieName("session"), Value: session})
	r.AddCookie(&http.Cookie{Name: s.cookieName("cli_flow"), Value: ticket})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("approve device: HTTP %d expected %d", w.Code, want)
	}
	if strings.Contains(w.Body.String(), d.DeviceCode) {
		t.Fatal("device secret exposed in browser")
	}
}
func deviceFixture(t *testing.T, repo Repository) (*Service, string) {
	t.Helper()
	s, _ := serviceFixture(t, repo)
	session, _, err := s.login(context.Background(), "directory", externalUser{Subject: "alice", Name: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	return s, session
}
func deviceContract(t *testing.T, repo Repository) {
	s, session := deviceFixture(t, repo)
	d, verifier := startDevice(t, s)
	decideDevice(t, s, session, d, "approve", "WRNG-CODE", 403)
	deviceCall(t, s, "/auth/cli/device/token", v1.CLIDevicePoll{DeviceCode: d.DeviceCode, CodeVerifier: randomToken()}, 403)
	decideDevice(t, s, session, d, "approve", d.UserCode, 200)
	var successes atomic.Int64
	var token string
	var mu sync.Mutex
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Go(func() {
			r := deviceCall(t, s, "/auth/cli/device/token", v1.CLIDevicePoll{DeviceCode: d.DeviceCode, CodeVerifier: verifier}, 200)
			if r.Status == "authorized" {
				successes.Add(1)
				mu.Lock()
				token = r.Token
				mu.Unlock()
			}
		})
	}
	group.Wait()
	if successes.Load() != 1 {
		t.Fatal("grant not single-use", successes.Load())
	}
	actor, err := s.CLIActor(context.Background(), token)
	if err != nil || len(actor.Grants) != 1 || actor.Grants["alpha"] != "owner" || actor.Grants["beta"] != "" {
		t.Fatal("device tenant/identity scope", err)
	}
	if err = s.revokeCLI(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CLIActor(context.Background(), token); !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrForbidden) {
		t.Fatal("revoked token accepted", err)
	}
}
func TestCLIDeviceSingleUseAndTenantScope(t *testing.T) { deviceContract(t, NewMemory()) }
func TestPostgresCLIDeviceSingleUseAndRestart(t *testing.T) {
	db := os.Getenv("TEST_DATABASE_URL")
	if db == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable database")
	}
	u, err := url.Parse(db)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", fmt.Sprintf("device_%d", time.Now().UnixNano()))
	u.RawQuery = q.Encode()
	repo, err := NewPostgres(context.Background(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	schema := repo.pool.Config().ConnConfig.RuntimeParams["search_path"]
	t.Cleanup(func() {
		repo.Close()
		c, e := pgx.Connect(context.Background(), db)
		if e == nil {
			defer func() { _ = c.Close(context.Background()) }()
			_, e = c.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		}
		if e != nil {
			t.Error(e)
		}
	})
	deviceContract(t, repo)
	s, session := deviceFixture(t, repo)
	d, v := startDevice(t, s)
	decideDevice(t, s, session, d, "approve", d.UserCode, 200)
	repo.Close()
	reopened, err := NewPostgres(context.Background(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	s.repo = reopened
	result := deviceCall(t, s, "/auth/cli/device/token", v1.CLIDevicePoll{DeviceCode: d.DeviceCode, CodeVerifier: v}, 200)
	if result.Status != "authorized" {
		t.Fatal("approved device grant lost on restart", result.Status)
	}
}
func TestCLIDevicePollingCancellationExpiryAndRevokedSession(t *testing.T) {
	for _, scenario := range []string{"throttle", "cancel", "expire", "session-revoked", "deny"} {
		t.Run(scenario, func(t *testing.T) {
			s, session := deviceFixture(t, NewMemory())
			at := time.Now()
			s.Now = func() time.Time { return at }
			d, v := startDevice(t, s)
			poll := v1.CLIDevicePoll{DeviceCode: d.DeviceCode, CodeVerifier: v}
			switch scenario {
			case "throttle":
				first := deviceCall(t, s, "/auth/cli/device/token", poll, 200)
				second := deviceCall(t, s, "/auth/cli/device/token", poll, 200)
				if first.Status != "authorization_pending" || second.Status != "slow_down" || second.IntervalSeconds != 10 {
					t.Fatal(first.Status, second.Status)
				}
				return
			case "cancel":
				deviceCall(t, s, "/auth/cli/device/cancel", poll, 200)
			case "expire":
				at = at.Add(6 * time.Minute)
			case "deny":
				decideDevice(t, s, session, d, "deny", "", 200)
			case "session-revoked":
				decideDevice(t, s, session, d, "approve", d.UserCode, 200)
				if e := s.repo.Update(context.Background(), "session", func(tx Tx) error { return tx.Delete("session", "global", hash(session)) }); e != nil {
					t.Fatal(e)
				}
			}
			result := deviceCall(t, s, "/auth/cli/device/token", poll, 200)
			if result.Status != "expired_token" && result.Status != "access_denied" {
				t.Fatal("inactive device granted token", result.Status)
			}
		})
	}
}
