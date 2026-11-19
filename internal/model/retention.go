package model

// MinimumHistorySeconds keeps useful chart history for accelerated demo and
// short-window objectives without changing objective or alert evaluation.
const MinimumHistorySeconds int64 = 24 * 60 * 60

// RetentionSeconds preserves the existing two-window horizon for longer SLOs.
// Retention is a storage bound, not a promise that missing history is available.
func (s SLO) RetentionSeconds() int64 {
	step := max(int64(1), s.StepSeconds)
	minimum := ((MinimumHistorySeconds + step - 1) / step) * step
	return max(minimum, 2*s.WindowSeconds)
}
