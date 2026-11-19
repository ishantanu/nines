package identity

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLIDevicePagePreservesExplicitConfirmationAndEscapesIdentity(t *testing.T) {
	var out bytes.Buffer
	err := cliPage.Execute(&out, map[string]any{"Authenticated": true, "Name": "<script>fixture</script>", "Role": "editor", "Tenant": "alpha", "Server": "http://127.0.0.1", "UserCode": "ABCD-2345", "Ticket": "fixture-ticket", "CSRF": "fixture-csrf"})
	if err != nil {
		t.Fatal(err)
	}
	page := out.String()
	for _, required := range []string{`class="device-code">ABCD-2345`, `id="device-user-code" name="userCode"`, `aria-describedby="device-code-help"`, `maxlength="9" required`, `action="/auth/cli/approve"`, `name="request" value="fixture-ticket"`, `name="csrf" value="fixture-csrf"`, `name="decision" value="deny"`, `name="decision" value="approve"`, `Only approve if you started`, `@media(max-width:420px)`} {
		if !strings.Contains(page, required) {
			t.Fatalf("device confirmation omitted %s", required)
		}
	}
	if strings.Contains(page, "<script>") || !strings.Contains(page, "&lt;script&gt;fixture&lt;/script&gt;") {
		t.Fatal("identity text was not HTML-escaped")
	}
}
