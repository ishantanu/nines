package main

import (
	"flag"
	"fmt"

	"github.com/ishantanu/nines/internal/features"
)

// flagSet reports whether a command-line flag was given explicitly.
func flagSet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

// serverFeatures resolves the feature gates. The synthetic demo shows every
// feature unless the operator names a list explicitly.
func serverFeatures(spec string, demo, explicit bool) (features.Set, error) {
	if demo && !explicit && spec == "" {
		return features.All(), nil
	}
	return features.Parse(spec)
}

// checkProviderGates refuses to start with a configured telemetry connection
// whose experimental provider is turned off, rather than silently ignoring it.
func checkProviderGates(runtimes []tenantRuntime, gates features.Set) error {
	for _, r := range runtimes {
		w := r.Workspace
		kinds := []string{w.Source.Kind}
		for _, c := range w.Connections {
			kinds = append(kinds, c.Kind)
		}
		if w.ManagedSources != nil {
			for _, c := range w.ManagedSources.List() {
				if kind, ok := c["kind"].(string); ok {
					kinds = append(kinds, kind)
				}
			}
		}
		for _, kind := range kinds {
			if kind != "" && !gates.ProviderEnabled(kind) {
				return fmt.Errorf("tenant %s uses the %s provider: %s", w.ID, kind, features.Disabled(kind))
			}
		}
	}
	return nil
}
