package datadog

import (
	"context"
	"os"
	"testing"
	"time"
)

// Opt-in read-only acceptance. IDs/bounds must identify disposable synthetic SLOs.
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
	c, e := New(os.Getenv("DD_API_URL"), os.Getenv("DD_API_KEY"), os.Getenv("DD_APP_KEY"))
	if e != nil {
		t.Fatal("configure Datadog test origin and credentials")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	out, e := c.NativeResults(ctx, os.Getenv("NINES_NATIVE_ID"), .99, from, to)
	if e != nil {
		t.Fatal(e)
	}
	if out.Attainment == nil || out.State != "observed" {
		t.Fatal("expected synthetic native traffic in selected range")
	}
	t.Logf("read-only native results: attainment=%.4f historyPoints=%d budgetPresent=%t", *out.Attainment, len(out.Points), out.BudgetRemaining != nil)
}
