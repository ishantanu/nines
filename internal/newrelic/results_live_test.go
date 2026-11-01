package newrelic

import (
	"context"
	"os"
	"testing"
	"time"
)

// Opt-in read-only acceptance. The parent entity/account is server-configured.
func TestLiveNativeResults(t *testing.T) {
	if os.Getenv("NINES_LIVE_NATIVE_READ") != "1" {
		t.Skip("explicit synthetic live native read opt-in required")
	}
	from, e := time.Parse(time.RFC3339, os.Getenv("NINES_NATIVE_FROM"))
	if e != nil {
		t.Fatal("set NINES_NATIVE_FROM")
	}
	to, e := time.Parse(time.RFC3339, os.Getenv("NINES_NATIVE_TO"))
	if e != nil {
		t.Fatal("set NINES_NATIVE_TO")
	}
	c, e := New(os.Getenv("NEWRELIC_API_URL"), os.Getenv("NEWRELIC_ACCOUNT_ID"), os.Getenv("NEWRELIC_API_KEY"))
	if e != nil {
		t.Fatal("configure New Relic test account and credentials")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	rows, e := c.NativeCatalog(ctx, os.Getenv("NINES_NATIVE_ENTITY"))
	if e != nil {
		t.Fatal(e)
	}
	for _, row := range rows {
		if row.ID != os.Getenv("NINES_NATIVE_ID") {
			continue
		}
		out, e := c.NativeResults(ctx, row, .99, from, to)
		if e != nil {
			t.Fatal(e)
		}
		if out.Attainment == nil || out.State != "observed" {
			t.Fatal("expected synthetic native traffic in selected range")
		}
		t.Logf("read-only native results: attainment=%.4f historyPoints=%d budgetPresent=%t", *out.Attainment, len(out.Points), out.BudgetRemaining != nil)
		return
	}
	t.Fatal("native synthetic SLO not found on configured entity")
}
