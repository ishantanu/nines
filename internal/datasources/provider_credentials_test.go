package datasources

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/identity"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

func TestManagedProviderCredentialsRoutesAndRestarts(t *testing.T) {
	for _, tc := range []struct {
		kind, url            string
		options, credentials map[string]string
	}{
		{"datadog", "https://api.uk1.datadoghq.com", nil, map[string]string{"apiKey": "private-dd-api", "applicationKey": "private-dd-app"}},
		{"newrelic", "https://api.eu.newrelic.com/graphql", map[string]string{"accountID": "123456"}, map[string]string{"userKey": "private-nr-user"}},
		{"dash0", "https://api.eu-west-1.aws.dash0.com", map[string]string{"dataset": "default"}, map[string]string{"bearerToken": "private-dash0-token"}},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			ctx := context.Background()
			repo := identity.NewMemory()
			st := store.NewMemory()
			create := func() *Registry {
				r, e := New(ctx, repo, "alpha", nil, map[string]bool{"api.uk1.datadoghq.com": true, "api.eu.newrelic.com": true, "api.eu-west-1.aws.dash0.com": true}, nil, base64.StdEncoding.EncodeToString(make([]byte, 32)), "", st, ProviderRoutes{"newrelic": {"123456"}, "dash0": {"default"}})
				if e != nil {
					t.Fatal(e)
				}
				return r
			}
			r := create()
			in := Input{Name: "trial", Kind: tc.kind, URL: tc.url, Options: copyOptions(tc.options), Credentials: tc.credentials, CollectionMode: "manual"}
			public, e := r.Save(ctx, in)
			if e != nil {
				t.Fatal(e)
			}
			if public["collectionMode"] != "manual" {
				t.Fatal("manual collection lost")
			}
			if _, ok := r.Target("trial"); ok {
				t.Fatal("unexpected rule publisher")
			}
			if e = repo.View(ctx, func(tx identity.Tx) error {
				raw, e := tx.Get("data_source", "alpha", "trial")
				for _, secret := range tc.credentials {
					if strings.Contains(string(raw), secret) || strings.Contains(string(encodeRecord(public)), secret) {
						t.Fatal("credential exposed")
					}
				}
				return e
			}); e != nil {
				t.Fatal(e)
			}
			restarted := create()
			source, ok := restarted.Source("trial")
			if !ok || source.CollectionMode != "manual" {
				t.Fatal("restart lost source")
			}
			stored := restarted.entries["trial"]
			decoded, e := restarted.credentials(stored.record)
			if e != nil {
				t.Fatal(e)
			}
			for name, secret := range tc.credentials {
				if decoded[name] != secret {
					t.Fatal("credential lost on restart")
				}
			}
			in.ExpectedVersion = 1
			in.Credentials = map[string]string{}
			for name := range tc.credentials {
				in.Credentials[name] = ""
				break
			}
			if _, e = r.Save(ctx, in); e != nil {
				t.Fatal("blank credential edit failed", e)
			}
			in.ExpectedVersion = 2
			in.ClearCredentials = true
			if _, e = r.Save(ctx, in); e == nil {
				t.Fatal("required credential clear accepted")
			}
			in.ClearCredentials = false
			objective := model.SLO{ID: "objective", Source: "trial", Name: "Test", Project: "p", Service: "api", Target: .99, StepSeconds: 60, WindowSeconds: 300}
			if e = st.Create(ctx, objective); e != nil {
				t.Fatal(e)
			}
			if tc.kind != "datadog" {
				if tc.kind == "newrelic" {
					in.Options = map[string]string{"accountID": "654321"}
				} else {
					in.Options = map[string]string{"dataset": "other"}
				}
				if _, e = r.Save(ctx, in); e == nil {
					t.Fatal("referenced route change accepted")
				}
			}
		})
	}
}
func TestManagedProviderRejectsUnapprovedRoutesAndPublisherFlags(t *testing.T) {
	ctx := context.Background()
	r, e := New(ctx, identity.NewMemory(), "alpha", nil, map[string]bool{"api.eu.newrelic.com": true, "api.eu-west-1.aws.dash0.com": true, "api.uk1.datadoghq.com": true}, nil, base64.StdEncoding.EncodeToString(make([]byte, 32)), "", store.NewMemory(), ProviderRoutes{"newrelic": {"123456"}, "dash0": {"default"}})
	if e != nil {
		t.Fatal(e)
	}
	for _, in := range []Input{
		{Name: "nr", Kind: "newrelic", URL: "https://api.eu.newrelic.com/graphql", Options: map[string]string{"accountID": "654321"}, Credentials: map[string]string{"userKey": "synthetic-user"}},
		{Name: "dash", Kind: "dash0", URL: "https://api.eu-west-1.aws.dash0.com", Options: map[string]string{"dataset": "other"}, BearerToken: "synthetic-token"},
		{Name: "dd", Kind: "datadog", URL: "https://api.uk1.datadoghq.com", Credentials: map[string]string{"apiKey": "synthetic-api", "applicationKey": "synthetic-app"}, RulesEnabled: true, AutoDeploy: true},
		{Name: "dd", Kind: "datadog", URL: "https://api.uk1.datadoghq.com", Credentials: map[string]string{"Authorization": "synthetic-value"}},
		{Name: "dd", Kind: "datadog", URL: "https://unapproved.example", Credentials: map[string]string{"apiKey": "synthetic-api", "applicationKey": "synthetic-app"}},
	} {
		if _, e = r.Save(ctx, in); e == nil {
			t.Fatal("unsafe managed source accepted", in.Kind)
		}
	}
}
func TestExistingEncryptedBearerRecordRemainsReadable(t *testing.T) {
	ctx := context.Background()
	repo := identity.NewMemory()
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	hosts := map[string]bool{"metrics.test": true}
	r, e := New(ctx, repo, "alpha", nil, hosts, nil, key, "", store.NewMemory())
	if e != nil {
		t.Fatal(e)
	}
	v := record{Name: "old", Kind: "prometheus", URL: "https://metrics.test", Version: 1}
	nonce := make([]byte, r.key.NonceSize())
	v.Secret = r.key.Seal(nonce, nonce, []byte("synthetic-legacy-bearer"), []byte("alpha/old"))
	if e = repo.Update(ctx, "legacy", func(tx identity.Tx) error { return tx.Put("data_source", "alpha", "old", encodeRecord(v), time.Time{}) }); e != nil {
		t.Fatal(e)
	}
	reopened, e := New(ctx, repo, "alpha", nil, hosts, nil, key, "", store.NewMemory())
	if e != nil {
		t.Fatal(e)
	}
	credentials, e := reopened.credentials(reopened.entries["old"].record)
	if e != nil || credentials["bearerToken"] != "synthetic-legacy-bearer" {
		t.Fatal("legacy bearer lost", e)
	}
}

func TestManagedDatadogNativePublicationOptInAndRestart(t *testing.T) {
	ctx := context.Background()
	repo := identity.NewMemory()
	st := store.NewMemory()
	open := func() *Registry {
		r, err := New(ctx, repo, "alpha", nil, map[string]bool{"api.uk1.datadoghq.com": true}, nil, base64.StdEncoding.EncodeToString(make([]byte, 32)), "", st)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := open()
	public, err := r.Save(ctx, Input{Name: "dd", Kind: "datadog", URL: "https://api.uk1.datadoghq.com", Credentials: map[string]string{"apiKey": "synthetic-api", "applicationKey": "synthetic-app"}, CollectionMode: "manual", RulesEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodeRecord(public)), "synthetic-") {
		t.Fatal("publisher credentials exposed")
	}
	target, ok := open().Target("dd")
	if !ok || target.Kind != "datadog" || target.AutoDeploy || target.DatadogAPIKey != "synthetic-api" || target.DatadogApplicationKey != "synthetic-app" {
		t.Fatal("native publisher lost on restart")
	}
}

func TestReferencedDatadogConnectionCanOptIntoReviewedPublication(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	r, err := New(ctx, identity.NewMemory(), "alpha", nil, map[string]bool{"api.uk1.datadoghq.com": true}, nil, base64.StdEncoding.EncodeToString(make([]byte, 32)), "", st)
	if err != nil {
		t.Fatal(err)
	}
	in := Input{Name: "dd", Kind: "datadog", URL: "https://api.uk1.datadoghq.com", Credentials: map[string]string{"apiKey": "synthetic-api", "applicationKey": "synthetic-app"}, CollectionMode: "manual"}
	if _, err = r.Save(ctx, in); err != nil {
		t.Fatal(err)
	}
	if err = st.Create(ctx, model.SLO{ID: "availability", Name: "Availability", Source: "dd", Project: "production", Service: "api", Target: .99, WindowSeconds: 604800, StepSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	in.Credentials = nil
	in.ExpectedVersion = 1
	in.RulesEnabled = true
	if _, err = r.Save(ctx, in); err != nil {
		t.Fatal("safe publication opt-in rejected", err)
	}
	target, ok := r.Target("dd")
	if !ok || target.AutoDeploy {
		t.Fatal("publication opt-in lost or enables automatic writes")
	}
	in.ExpectedVersion = 2
	in.RulesEnabled = false
	if _, err = r.Save(ctx, in); err == nil {
		t.Fatal("referenced publication route disabled without cleanup")
	}
}

func TestManagedNewRelicPublicationRequiresApprovedEntity(t *testing.T) {
	ctx := context.Background()
	repo := identity.NewMemory()
	st := store.NewMemory()
	open := func() *Registry {
		r, err := New(ctx, repo, "alpha", nil, map[string]bool{"api.eu.newrelic.com": true}, nil, base64.StdEncoding.EncodeToString(make([]byte, 32)), "", st, ProviderRoutes{"newrelic": {"123456"}, "newrelic-entity": {"approved-entity", "second-entity"}})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := open()
	in := Input{Name: "nr", Kind: "newrelic", URL: "https://api.eu.newrelic.com/graphql", Options: map[string]string{"accountID": "123456"}, Credentials: map[string]string{"userKey": "synthetic-user"}, CollectionMode: "manual"}
	if _, err := r.Save(ctx, in); err != nil {
		t.Fatal(err)
	}
	if err := st.Create(ctx, model.SLO{ID: "availability", Name: "Availability", Source: "nr", Project: "production", Service: "api", Target: .99, WindowSeconds: 86400, StepSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	in.ExpectedVersion = 1
	in.Credentials = nil
	in.RulesEnabled = true
	for _, guid := range []string{"", "unapproved-entity"} {
		in.NativeEntityGUID = guid
		if _, err := r.Save(ctx, in); err == nil {
			t.Fatal("unapproved native entity accepted", guid)
		}
	}
	in.NativeEntityGUID = "approved-entity"
	public, err := r.Save(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodeRecord(public)), "synthetic-user") {
		t.Fatal("native publisher credential leaked")
	}
	target, ok := open().Target("nr")
	if !ok || target.NewRelicEntityGUID != "approved-entity" || target.NewRelicAccountID != "123456" || target.NewRelicUserKey != "synthetic-user" || target.AutoDeploy {
		t.Fatal("native destination not restored")
	}
	in.ExpectedVersion = 2
	in.NativeEntityGUID = "second-entity"
	if _, err := r.Save(ctx, in); err == nil {
		t.Fatal("referenced native entity route changed")
	}
	in.NativeEntityGUID = "approved-entity"
	in.AutoDeploy = true
	if _, err := r.Save(ctx, in); err == nil {
		t.Fatal("automatic native publication accepted")
	}
}
