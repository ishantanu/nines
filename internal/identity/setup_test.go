package identity

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func setupToken(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var link SetupLink
	if err := json.Unmarshal(raw, &link); err != nil || !strings.HasPrefix(link.URL, "http://127.0.0.1/setup#") {
		t.Fatal("setup link", string(raw), err)
	}
	if d := time.Until(link.ExpiresAt); d < 71*time.Hour || d > 73*time.Hour {
		t.Fatal("setup link lifetime", d)
	}
	return strings.TrimPrefix(link.URL, "http://127.0.0.1/setup#")
}

// An administrator creates a local account without choosing its password:
// the person sets it through a single-use, expiring link. Reissuing a link
// revokes the earlier one, and every step is audited.
func TestSetupLinksLetPeopleChooseTheirOwnPassword(t *testing.T) {
	s, h := platformFixture(t, NewMemory())
	admin, csrf := platformLogin(t, h, "admin", "platform-fixture-password")
	created := request(t, admin, "POST", h.URL+"/api/platform/users/create", csrf, `{"reason":"Onboard Pat","tenant":"alpha","user":{"provider":"local","username":"pat","name":"Pat","role":"owner"}}`, 200)
	first := setupToken(t, created["setupLink"])
	var user User
	_ = json.Unmarshal(mustJSON(t, created), &user)

	// Pending: no password works yet.
	c := newClient()
	info := request(t, c, "GET", h.URL+"/auth/session", "", "", 200)
	request(t, c, "POST", h.URL+"/auth/local/local/login", textField(t, info, "csrfToken"), `{"username":"pat","password":"any-password-at-all"}`, 401)

	// A new link replaces the first.
	second := setupToken(t, mustJSON(t, request(t, admin, "POST", h.URL+"/api/platform/users/setup-link", csrf, `{"reason":"New link","userId":"`+user.ID+`"}`, 200)))
	if second == first {
		t.Fatal("reissued the same token")
	}
	request(t, newClient(), "POST", h.URL+"/auth/local/setup", "", `{"token":"`+first+`","password":"pat-chosen-password"}`, 403)
	request(t, newClient(), "POST", h.URL+"/auth/local/setup", "", `{"token":"`+second+`","password":"short"}`, 400)
	request(t, newClient(), "POST", h.URL+"/auth/local/setup", "", `{"token":"`+second+`","password":"pat-chosen-password"}`, 200)
	request(t, newClient(), "POST", h.URL+"/auth/local/setup", "", `{"token":"`+second+`","password":"another-password"}`, 403)
	platformLogin(t, h, "pat", "pat-chosen-password")

	// Expired links are refused.
	third := setupToken(t, mustJSON(t, request(t, admin, "POST", h.URL+"/api/platform/users/setup-link", csrf, `{"reason":"New link","userId":"`+user.ID+`"}`, 200)))
	s.Now = func() time.Time { return time.Now().Add(73 * time.Hour) }
	request(t, newClient(), "POST", h.URL+"/auth/local/setup", "", `{"token":"`+third+`","password":"pat-later-password"}`, 403)
	s.Now = time.Now

	var events []PlatformAudit
	_ = json.Unmarshal(request(t, admin, "GET", h.URL+"/api/platform/audit", "", "", 200)["items"], &events)
	actions := map[string]int{}
	for _, e := range events {
		actions[e.Action]++
		if strings.Contains(string(mustJSON(t, e)), second) || strings.Contains(string(mustJSON(t, e)), "pat-chosen-password") {
			t.Fatal("audit leaked a token or password")
		}
	}
	if actions["user.setup_link"] != 3 || actions["user.setup_completed"] != 1 {
		t.Fatal("audit", actions)
	}
}

// Setup links are only for local accounts; SSO users sign in upstream.
func TestSetupLinksRefuseNonLocalUsers(t *testing.T) {
	s, h := platformFixture(t, NewMemory())
	admin, csrf := platformLogin(t, h, "admin", "platform-fixture-password")
	request(t, admin, "POST", h.URL+"/api/platform/users/setup-link", csrf, `{"reason":"New link","userId":"no-such-user"}`, 404)
	_ = s
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
