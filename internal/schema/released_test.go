package schema_test

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/identity"
	"github.com/ishantanu/nines/internal/intentflow"
	"github.com/ishantanu/nines/internal/investigation"
	"github.com/ishantanu/nines/internal/replay"
	"github.com/ishantanu/nines/internal/schema"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/workflow"
)

var components = map[string][]schema.Migration{
	"store":         store.Migrations,
	"identity":      identity.Migrations,
	"intent":        intentflow.Migrations,
	"investigation": investigation.Migrations,
	"replay":        replay.Migrations,
	"workflow":      workflow.Migrations,
}

// released.txt records every migration that has shipped, with its checksum.
// A released migration must never change: databases already ran it and will
// not run it again. Add a new migration instead. Run with
// RECORD_MIGRATIONS=1 to append new migrations; existing lines never change.
func TestReleasedMigrationsAreUnchanged(t *testing.T) {
	recorded := map[string]string{}
	var lines []string
	if f, err := os.Open("released.txt"); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			lines = append(lines, line)
			if fields := strings.Fields(line); len(fields) == 3 && !strings.HasPrefix(line, "#") {
				recorded[fields[0]+" "+fields[1]] = fields[2]
			}
		}
		_ = f.Close()
	}
	var missing []string
	for component, ms := range components {
		if err := schema.Validate(component, ms); err != nil {
			t.Fatal(err)
		}
		for _, m := range ms {
			key := fmt.Sprintf("%s %d", component, m.ID)
			got, ok := recorded[key]
			switch {
			case !ok:
				missing = append(missing, fmt.Sprintf("%s %s", key, m.Checksum()))
			case got != m.Checksum():
				t.Fatalf("released migration %s (%s) changed. Databases that already ran it will not run it again; restore it and add a new migration instead", key, m.Name)
			}
		}
	}
	if len(missing) == 0 {
		return
	}
	if os.Getenv("RECORD_MIGRATIONS") != "1" {
		t.Fatalf("migrations not recorded in released.txt: %v; run RECORD_MIGRATIONS=1 go test ./internal/schema", missing)
	}
	lines = append(lines, missing...)
	if err := os.WriteFile("released.txt", []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
