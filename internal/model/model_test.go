package model

import (
	"math"
	"testing"
	"time"
)

func TestSampleAndConfigurationValidation(t *testing.T) {
	at := time.Unix(1800000000, 0)
	for _, p := range []Point{{Time: at, Good: 2, Total: 1}, {Time: at, Total: -1}, {Time: at, Total: math.Inf(1)}, {Time: at, Good: math.NaN(), Total: 1}, {Time: at.Add(time.Second), Total: 1}, {Time: at.Add(time.Nanosecond), Total: 1}, {Time: at, Total: 1e100}, {Time: at, Total: 1e-100}} {
		if err := p.Validate(60); err == nil {
			t.Errorf("accepted invalid point %+v", p)
		}
	}
	s := SLO{ID: "valid", Project: "p", Service: "s", Name: "Valid", Target: .999, WindowSeconds: 3600, StepSeconds: 60}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, target := range []float64{0, 1, math.NaN(), math.Inf(1)} {
		bad := s
		bad.Target = target
		if err := bad.Validate(); err == nil {
			t.Fatal("accepted invalid target", target)
		}
	}
	bad := s
	bad.Policy = &Policy{ShortWindowSeconds: 300, LongWindowSeconds: 7200, BurnRate: 14.4, MinCoverage: 1}
	if err := bad.Validate(); err == nil {
		t.Fatal("accepted alert window beyond retention window")
	}
}
