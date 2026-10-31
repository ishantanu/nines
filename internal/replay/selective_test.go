package replay

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestSelectiveLabelsAreBoundedAndIdempotent(t *testing.T) {
	raw, err := os.ReadFile("testdata/selective-events.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []json.RawMessage
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	clean, err := Sanitize(events)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(clean)
	if strings.Contains(string(payload), "secret") {
		t.Fatal("private text leaked")
	}
	var snapshot map[string]any
	_ = json.Unmarshal(clean[1], &snapshot)
	body := asMap(asMap(snapshot["data"])["node"])["childNodes"].([]any)[0].(map[string]any)["childNodes"].([]any)
	text := func(index int) string {
		return asMap(asMap(body[index])["childNodes"].([]any)[0])["textContent"].(string)
	}
	if text(0) != "Home" || asMap(body[1])["textContent"] != "****" || text(5) != "****" || text(6) != "No SLI data" {
		t.Fatal("snapshot annotation boundary failed")
	}
	var mutation map[string]any
	_ = json.Unmarshal(clean[2], &mutation)
	data := asMap(mutation["data"])
	got := []string{}
	for _, v := range data["texts"].([]any) {
		got = append(got, asMap(v)["value"].(string))
	}
	want := []string{"Home", "****", "*********************", "No SLI data"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("texts = %v", got)
	}
	adds := data["adds"].([]any)
	if asMap(asMap(adds[0])["node"])["textContent"] != "Rules" || asMap(asMap(adds[1])["node"])["textContent"] != "****" {
		t.Fatal("added text scope failed")
	}
	if strings.Contains(string(clean[3]), "Home") || strings.Contains(string(clean[4]), "Home") || strings.Contains(string(clean[5]), "Home") {
		t.Fatal("input or reset scope leaked")
	}
	again, err := Sanitize(clean)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(clean, again) {
		t.Fatal("sanitization not idempotent")
	}
	legacy, err := sanitize(events, false)
	if err != nil {
		t.Fatal(err)
	}
	legacyBytes, _ := json.Marshal(legacy)
	if strings.Contains(string(legacyBytes), "Home") || strings.Contains(string(legacyBytes), "data-replay-label") {
		t.Fatal("legacy policy changed")
	}
}

func TestRecordingConsentPolicyIsPreserved(t *testing.T) {
	modes(t, func(t *testing.T, pg bool) {
		svc, ctx, _ := fixture(t, pg)
		for i, policy := range []string{LegacyPolicy, Policy} {
			in := recording()
			in.ID = []string{"legacy-policy", "selective-policy"}[i]
			in.Policy = policy
			saved, err := svc.Create(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			if saved.Policy != policy {
				t.Fatalf("policy = %q", saved.Policy)
			}
			retry, err := svc.Create(ctx, in)
			if err != nil || retry.Digest != saved.Digest {
				t.Fatalf("idempotent retry: %v", err)
			}
			if policy == Policy {
				in.Policy = LegacyPolicy
			} else {
				in.Policy = Policy
			}
			if _, err := svc.Create(ctx, in); err != ErrConflict {
				t.Fatalf("changed consent retry = %v", err)
			}
		}
	})
}

func TestReplaySeparateSharingConsentAndPermanentDeletion(t *testing.T) {
	modes(t, func(t *testing.T, pg bool) {
		svc, ctx, _ := fixture(t, pg)
		in := recording()
		for _, change := range []func(*v1.CreateSessionReplay){func(r *v1.CreateSessionReplay) { r.SharingConsent = false }, func(r *v1.CreateSessionReplay) { r.NoticeVersion = "stale-notice" }} {
			bad := in
			change(&bad)
			if _, e := svc.Create(ctx, bad); e == nil {
				t.Fatal("implicit/stale sharing consent accepted")
			}
		}
		saved, e := svc.Create(ctx, in)
		if e != nil || saved.ConsentReceipt == nil || saved.ConsentReceipt.Actor.Subject == "" || saved.ConsentReceipt.NoticeVersion != v1.ReplayConsentNotice {
			t.Fatal(saved, e)
		}
		if e = svc.Delete(ctx, saved.ID, saved.Version+1); e != ErrConflict {
			t.Fatal("stale deletion accepted", e)
		}
		if e = svc.Delete(ctx, saved.ID, saved.Version); e != nil {
			t.Fatal(e)
		}
		if _, e = svc.Events(ctx, saved.ID); e != ErrNotFound {
			t.Fatal("deleted event payload retained", e)
		}
		if _, e = svc.Read(ctx, saved.ID); e != ErrNotFound {
			t.Fatal("deleted metadata retained", e)
		}
		svc.Privacy.Disabled = true
		if _, e = svc.Create(ctx, in); e == nil {
			t.Fatal("disabled replay accepted")
		}
	})
}
