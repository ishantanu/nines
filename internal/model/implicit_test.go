package model

import (
	"strings"
	"testing"
)

func TestImplicitTargetNotesComparesChangesWithDeliveredReliability(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	has := func(notes []string, fragment string) bool {
		for _, n := range notes {
			if strings.Contains(n, fragment) {
				return true
			}
		}
		return false
	}
	if notes := ImplicitTargetNotes(0.99, 0.99, f(0.999), 1); notes != nil {
		t.Fatalf("an unchanged target needs no note: %v", notes)
	}
	// Tightening above what is delivered would be unmet immediately.
	if n := ImplicitTargetNotes(0.99, 0.995, f(0.992), 1); !has(n, "start out unmet") {
		t.Fatalf("%v", n)
	}
	// Tightening to a level already delivered is fine and silent.
	if n := ImplicitTargetNotes(0.99, 0.995, f(0.9995), 1); len(n) != 0 {
		t.Fatalf("%v", n)
	}
	// Relaxing far below what users get is called out, with the multiple.
	if n := ImplicitTargetNotes(0.9995, 0.99, f(0.9999), 1); !has(n, "allows about 100 times more failures") || !has(n, "communicate") {
		t.Fatalf("%v", n)
	}
	// A modest relaxation near delivered performance is not.
	if n := ImplicitTargetNotes(0.999, 0.995, f(0.996), 1); has(n, "allows about") {
		t.Fatalf("%v", n)
	}
	// A tenfold change in the allowed failure rate is called large either way.
	if n := ImplicitTargetNotes(0.999, 0.9999, f(0.99995), 1); !has(n, "tightens the allowed failure rate by 10 times") {
		t.Fatalf("%v", n)
	}
	if n := ImplicitTargetNotes(0.9999, 0.999, nil, 1); !has(n, "relaxes the allowed failure rate by 10 times") || !has(n, "not fully measured") {
		t.Fatalf("%v", n)
	}
	// Partial coverage cannot establish what users received.
	if n := ImplicitTargetNotes(0.99, 0.995, f(0.999), 0.5); !has(n, "not fully measured") {
		t.Fatalf("%v", n)
	}
	// A perfect window must not divide by zero.
	if n := ImplicitTargetNotes(0.999, 0.99, f(1), 1); !has(n, "allows about") {
		t.Fatalf("%v", n)
	}
	// Out-of-range targets are left to validation.
	if n := ImplicitTargetNotes(0, 1, f(1), 1); n != nil {
		t.Fatalf("%v", n)
	}
}
