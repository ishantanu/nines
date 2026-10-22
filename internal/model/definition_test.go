package model_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/model"
)

func TestSharedDefinitionSchemaAndCanonicalDigest(t *testing.T) {
	for _, body := range []string{examples.APIAvailability, examples.Checkout} {
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
	d, err := model.ParseSLODefinition([]byte(examples.APIAvailability))
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
		"version":               strings.Replace(examples.APIAvailability, model.APIVersion, "reliability.platform/v2", 1),
		"kind":                  strings.Replace(examples.APIAvailability, model.SLOKind, "Objective", 1),
		"unknown-spec":          examples.APIAvailability + "  typo: 1\n",
		"unknown-metadata":      strings.Replace(examples.APIAvailability, "  project:", "  typo: value\n  project:", 1),
		"duplicate":             examples.APIAvailability + "  target: 0.8\n",
		"multiple-documents":    examples.APIAvailability + "\n---\n" + examples.Checkout,
		"flat-runtime-json":     `{"id":"old","project":"p","name":"old","target":0.99}`,
		"size":                  strings.Repeat(" ", (1<<20)+1),
		"empty":                 "",
		"invalid-policy":        strings.Replace(examples.APIAvailability, "minCoverage: 1", "minCoverage: 0", 1),
		"unknown-journey-field": examples.Checkout + "  typo: 1\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := model.ParseDefinition([]byte(body)); err == nil {
				t.Fatal("invalid definition accepted")
			}
		})
	}
}
