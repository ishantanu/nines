package model_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/model"
)

// The v1alpha1 forms of the shipped examples, frozen when the examples moved to
// v1beta1. These tests pin v1alpha1 parsing and digest identity.
var alphaAPIAvailability, alphaCheckout = func() (string, string) {
	read := func(name string) string {
		raw, err := os.ReadFile("../../api/v1beta1/testdata/v1alpha1/" + name)
		if err != nil {
			panic(err)
		}
		return string(raw)
	}
	return read("api-availability.slo.yaml"), read("checkout.journey.yaml")
}()

func TestSharedDefinitionSchemaAndCanonicalDigest(t *testing.T) {
	for _, body := range []string{alphaAPIAvailability, alphaCheckout} {
		d, err := model.ParseDefinition([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		b, err := yaml.Marshal(d.Manifest())
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := model.ParseDefinition(b)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(d, parsed) || model.DefinitionDigest(d.Manifest()) != model.DefinitionDigest(parsed.Manifest()) {
			t.Fatal("export changed definition")
		}
		commented, err := model.ParseDefinition([]byte("# reformatted\n" + body))
		if err != nil || model.DefinitionDigest(d.Manifest()) != model.DefinitionDigest(commented.Manifest()) {
			t.Fatal("formatting changed digest", err)
		}
	}
	d, err := model.ParseSLODefinition([]byte(alphaAPIAvailability))
	if err != nil {
		t.Fatal(err)
	}
	runtime := d.SLO()
	runtime.CreatedAt = time.Now()
	if !reflect.DeepEqual(model.DefinitionFromSLO(runtime), d) {
		t.Fatal("runtime fields leaked or policy lost")
	}
}

func TestStrictDefinitionRejection(t *testing.T) {
	for name, body := range map[string]string{
		"version":               strings.Replace(alphaAPIAvailability, model.APIVersion, "reliability.platform/v2", 1),
		"kind":                  strings.Replace(alphaAPIAvailability, model.SLOKind, "Objective", 1),
		"unknown-spec":          alphaAPIAvailability + "  typo: 1\n",
		"unknown-metadata":      strings.Replace(alphaAPIAvailability, "  project:", "  typo: value\n  project:", 1),
		"duplicate":             alphaAPIAvailability + "  target: 0.8\n",
		"multiple-documents":    alphaAPIAvailability + "\n---\n" + alphaCheckout,
		"flat-runtime-json":     `{"id":"old","project":"p","name":"old","target":0.99}`,
		"size":                  strings.Repeat(" ", (1<<20)+1),
		"empty":                 "",
		"invalid-policy":        strings.Replace(alphaAPIAvailability, "minCoverage: 1", "minCoverage: 0", 1),
		"unknown-journey-field": alphaCheckout + "  typo: 1\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := model.ParseDefinition([]byte(body)); err == nil {
				t.Fatal("invalid definition accepted")
			}
		})
	}
}

// The shipped v1beta1 examples parse, and v1beta1 parsing is as strict as
// v1alpha1's.
func TestShippedV1beta1ExamplesParseStrictly(t *testing.T) {
	for _, body := range []string{examples.APIAvailability, examples.Checkout, examples.AgentResolution} {
		d, err := model.ParseDefinition([]byte(body))
		if err != nil || d.SourceAPIVersion != "nines.dev/v1beta1" {
			t.Fatal(d.SourceAPIVersion, err)
		}
	}
	for name, body := range map[string]string{
		"unknown-spec":       examples.APIAvailability + "  typo: 1\n",
		"duplicate":          examples.APIAvailability + "  target: 0.8\n",
		"multiple-documents": examples.APIAvailability + "\n---\n" + examples.Checkout,
		"alpha-field":        examples.APIAvailability + "  windowSeconds: 86400\n",
		"journey-typo":       examples.Checkout + "  typo: 1\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := model.ParseDefinition([]byte(body)); err == nil {
				t.Fatal("invalid definition accepted")
			}
		})
	}
}

func TestDurationYAMLRetainsNumericDefinitionDigest(t *testing.T) {
	readable, err := model.ParseDefinition([]byte(alphaAPIAvailability))
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := json.Marshal(readable.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := model.ParseDefinition(runtime)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(readable, legacy) || model.DefinitionDigest(readable.Manifest()) != model.DefinitionDigest(legacy.Manifest()) {
		t.Fatal("duration spelling changed policy identity")
	}
	yamlBody, err := yaml.Marshal(legacy.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(yamlBody), "shortWindowSeconds:") || !strings.Contains(string(yamlBody), "shortWindow: 5m") {
		t.Fatal("noncanonical duration export", string(yamlBody))
	}
	seconds, err := model.ParseDefinition([]byte(strings.ReplaceAll(string(yamlBody), "shortWindow: 5m", "shortWindow: 300s")))
	if err != nil {
		t.Fatal(err)
	}
	if model.DefinitionDigest(seconds.Manifest()) != model.DefinitionDigest(readable.Manifest()) {
		t.Fatal("equivalent durations changed digest")
	}
}
