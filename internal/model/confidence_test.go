package model

import (
	"math"
	"testing"
)

func TestZScoreMatchesStandardTables(t *testing.T) {
	for confidence, want := range map[float64]float64{0.90: 1.6448536, 0.95: 1.9599640, 0.99: 2.5758293, 0.999: 3.2905267} {
		if got := ZScore(confidence); math.Abs(got-want) > 1e-6 {
			t.Errorf("ZScore(%v) = %v, want %v", confidence, got, want)
		}
	}
}

func TestConfidentErrorRateRaisesThresholdAsTrafficFalls(t *testing.T) {
	z := ZScore(0.99)
	p := 14.4 * 0.01 // a 14.4x burn on a 99% target
	// Failures needed to fire at 99% confidence for 1000, 200 and 100 events.
	for n, want := range map[float64]float64{1000: 173, 200: 42, 100: 24} {
		got := math.Ceil(ConfidentErrorRate(p, n, z) * n)
		if got != want {
			t.Errorf("failures needed for %v events = %v, want %v", n, got, want)
		}
	}
	if big := ConfidentErrorRate(p, 1e9, z); math.Abs(big-p) > 1e-3 {
		t.Errorf("a busy service must converge to the plain threshold: %v vs %v", big, p)
	}
	if !math.IsInf(ConfidentErrorRate(p, 0, z), 1) {
		t.Error("no events can never meet a confidence threshold")
	}
}

func TestBurnThresholdMetOnlyAddsStrictnessWhenConfigured(t *testing.T) {
	plain := Policy{BurnRate: 14.4}
	// 2 failures in 10 events is a 20% error rate: a 20x burn on a 99% target.
	if !BurnThresholdMet(plain, 0.99, 2, 10, 20) {
		t.Fatal("the plain threshold must keep its behavior")
	}
	confident := Policy{BurnRate: 14.4, Confidence: 0.99}
	if BurnThresholdMet(confident, 0.99, 2, 10, 20) {
		t.Fatal("two failures in ten events is not confident evidence of a 14.4x burn")
	}
	if !BurnThresholdMet(confident, 0.99, 600, 1000, 60) {
		t.Fatal("a large, clear breach must still fire")
	}
	if BurnThresholdMet(confident, 0.99, 0, 0, 100) {
		t.Fatal("no traffic must never fire")
	}
}
