package model

import "math"

// Confidence-adjusted burn alerting for low-traffic services.
//
// With few events, a burn rate can cross its threshold by chance. When a
// policy sets Confidence, each window must show an observed error rate of at
// least P + z*sqrt(P*(1-P)/N), where P = burnRate*(1-target) is the error rate
// the burn threshold represents, N is the events in that window and z is the
// two-tailed normal score for the confidence. As N grows the extra term shrinks
// to nothing, so busy services behave exactly as before. The rule generators
// and the engine call these same functions, so they cannot disagree.

// ZScore returns the two-tailed standard normal score for a confidence in
// (0,1), for example 0.99 gives about 2.576. It uses Acklam's rational
// approximation of the inverse normal CDF, accurate to about 1e-9.
func ZScore(confidence float64) float64 {
	return inverseNormalCDF((1 + confidence) / 2)
}

func inverseNormalCDF(p float64) float64 {
	a := [...]float64{-3.969683028665376e+01, 2.209460984245205e+02, -2.759285104469687e+02, 1.383577518672690e+02, -3.066479806614716e+01, 2.506628277459239e+00}
	b := [...]float64{-5.447609879822406e+01, 1.615858368580409e+02, -1.556989798598866e+02, 6.680131188771972e+01, -1.328068155288572e+01}
	c := [...]float64{-7.784894002430293e-03, -3.223964580411365e-01, -2.400758277161838e+00, -2.549732539343734e+00, 4.374664141464968e+00, 2.938163982698783e+00}
	d := [...]float64{7.784695709041462e-03, 3.224671290700398e-01, 2.445134137142996e+00, 3.754408661907416e+00}
	const low, high = 0.02425, 1 - 0.02425
	switch {
	case p < low:
		q := math.Sqrt(-2 * math.Log(p))
		return (((((c[0]*q+c[1])*q+c[2])*q+c[3])*q+c[4])*q + c[5]) / ((((d[0]*q+d[1])*q+d[2])*q+d[3])*q + 1)
	case p > high:
		q := math.Sqrt(-2 * math.Log(1-p))
		return -(((((c[0]*q+c[1])*q+c[2])*q+c[3])*q+c[4])*q + c[5]) / ((((d[0]*q+d[1])*q+d[2])*q+d[3])*q + 1)
	default:
		q := p - 0.5
		r := q * q
		return (((((a[0]*r+a[1])*r+a[2])*r+a[3])*r+a[4])*r + a[5]) * q / (((((b[0]*r+b[1])*r+b[2])*r+b[3])*r+b[4])*r + 1)
	}
}

// ConfidentErrorRate is the lowest observed error rate that meets the burn
// threshold with the policy's confidence over n events. It is never below the
// plain threshold p, and above 1 means no observation can reach it.
func ConfidentErrorRate(p float64, n float64, z float64) float64 {
	if n <= 0 {
		return math.Inf(1)
	}
	return p + z*math.Sqrt(p*(1-p)/n)
}

// BurnThresholdMet reports whether a window's observed counts meet a policy's
// burn threshold. Without Confidence this is the plain burn-rate comparison.
func BurnThresholdMet(p Policy, target float64, bad, total float64, burnRate float64) bool {
	if burnRate < p.BurnRate {
		return false
	}
	if p.Confidence == 0 {
		return true
	}
	threshold := ConfidentErrorRate(p.BurnRate*(1-target), total, ZScore(p.Confidence))
	return total > 0 && bad/total >= threshold
}
