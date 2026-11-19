package migrate

import (
	"os"
	"strings"
	"testing"
)

// These fixtures are real, published example files from each project. They
// guard the importers against shapes our own samples might not exercise.

func TestOfficialSlothGettingStartedFileConverts(t *testing.T) {
	data, err := os.ReadFile("testdata/sloth-getting-started.yml")
	if err != nil {
		t.Fatal(err)
	}
	r, err := Convert("sloth", data, Options{})
	if err != nil || len(r.Definitions) != 1 || len(r.Skipped) != 0 {
		t.Fatalf("%+v %v", r, err)
	}
	slo := definition(t, r.Definitions[0].YAML)
	if slo.Target != 0.999 || slo.Labels["owner"] != "myteam" || slo.Labels["repo"] != "myorg/myservice" {
		t.Fatalf("%+v", slo)
	}
	// The regex matcher survives, including its parentheses and alternation.
	if !strings.Contains(slo.GoodQuery, `code=~"(5..|429)"`) {
		t.Fatalf("%s", slo.GoodQuery)
	}
	notes := ""
	for _, n := range r.Definitions[0].Notes {
		notes += n.Level + ":" + n.Field + " "
	}
	// This file sets alert names, labels and annotations; the report must say they are dropped.
	if !strings.Contains(notes, "lossy:alerting") || !strings.Contains(notes, "lossy:description") {
		t.Fatal(notes)
	}
}

func TestOfficialPyrraExamplesConvertIncludingEmptyDescriptionsAndLeadingSeparators(t *testing.T) {
	data, err := os.ReadFile("testdata/pyrra-examples.yaml")
	if err != nil {
		t.Fatal(err)
	}
	r, err := Convert("pyrra", data, Options{})
	if err != nil || len(r.Definitions) != 3 || len(r.Skipped) != 0 {
		t.Fatalf("%+v %v", r, err)
	}
	byName := map[string]string{}
	for _, d := range r.Definitions {
		byName[d.Name] = d.YAML
	}
	lat := definition(t, byName["apiserver-read-resource-latency"])
	if lat.WindowSeconds != 14*86400 || lat.Target != 0.99 || lat.Service != "monitoring" || !strings.Contains(lat.GoodQuery, `le="0.1"`) {
		t.Fatalf("%+v", lat)
	}
	errs := definition(t, byName["kubelet-request-errors"])
	if !strings.Contains(errs.GoodQuery, `code=~"5.."`) || errs.Labels["role"] != "alert-rules" {
		t.Fatalf("%+v", errs)
	}
	// The namespace stood in for the service, and the report says so.
	var said bool
	for _, n := range r.Definitions[0].Notes {
		said = said || (n.Field == "service" && strings.Contains(n.Message, "namespace"))
	}
	if !said {
		t.Fatal("a namespace used as the service must be reported")
	}
	// An empty description is not reported as lost information.
	for _, d := range r.Definitions {
		for _, n := range d.Notes {
			if n.Field == "description" {
				t.Fatalf("an empty description is not a loss: %+v", n)
			}
		}
	}
}
