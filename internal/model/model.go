package model

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	v1alpha1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/promqlvalidate"
)

var (
	ErrJourneyNotFound  = errors.New("journey or revision not found")
	ErrRevisionConflict = errors.New("resource revision changed; fetch the latest revision and retry")
	ErrRunConflict      = errors.New("run ID already exists with different evidence")
	ErrNotFound         = errors.New("SLO not found")
	ErrConflict         = errors.New("SLO already exists")
	ErrUnavailable      = errors.New("integration is not configured")
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string       { return e.Message }
func Invalid(format string, args ...any) error { return &ValidationError{fmt.Sprintf(format, args...)} }

type Policy = v1alpha1.BurnRatePolicy

type SLO struct {
	Measurement        *v1alpha1.MeasurementSpec  `json:"measurement,omitempty"`
	Intent             *v1alpha1.IntentSpec       `json:"intent,omitempty"`
	ProviderWindow     *v1alpha1.ProviderWindow   `json:"providerWindow,omitempty"`
	MeasurementBinding *v1alpha1.BindingReference `json:"measurementBinding,omitempty"`
	Source             string                     `json:"source,omitempty"`
	Revision           int64                      `json:"revision,omitempty"`
	ID                 string                     `json:"id"`
	Project            string                     `json:"project"`
	Service            string                     `json:"service"`
	Name               string                     `json:"name"`
	Target             float64                    `json:"target"`
	WindowSeconds      int64                      `json:"windowSeconds"`
	StepSeconds        int64                      `json:"stepSeconds"`
	GoodQuery          string                     `json:"goodQuery,omitempty"`
	TotalQuery         string                     `json:"totalQuery,omitempty"`
	Policies           []Policy                   `json:"policies,omitempty"`
	Policy             *Policy                    `json:"policy,omitempty"`
	CreatedAt          time.Time                  `json:"createdAt"`
}

var counterSelector = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*(\{([a-zA-Z_][a-zA-Z0-9_]*="([^"\\]|\\.)*"(,[a-zA-Z_][a-zA-Z0-9_]*="([^"\\]|\\.)*")*)?\})?$`)

var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func (s SLO) HasQueries() bool { return s.ProviderWindow != nil || s.GoodQuery != "" }
func (s SLO) Validate() error {
	if s.Measurement != nil {
		if err := ValidateInlineSLO(s); err != nil {
			return err
		}
	}
	if err := validateSLOIntent(s); err != nil {
		return err
	}
	if p := s.ProviderWindow; p != nil {
		if s.GoodQuery != "" || s.TotalQuery != "" {
			return Invalid("providerWindow cannot be combined with interval queries")
		}
		if p.ScrapeIntervalSeconds < 1 || p.ScrapeIntervalSeconds > s.StepSeconds/2 || s.StepSeconds%p.ScrapeIntervalSeconds != 0 {
			return Invalid("scrapeIntervalSeconds must divide stepSeconds with at least two scrapes per step")
		}
		for _, q := range []struct{ field, text string }{{"providerWindow.goodSelector", p.GoodSelector}, {"providerWindow.totalSelector", p.TotalSelector}} {
			if !counterSelector.MatchString(q.text) || len(q.text) > 8000 {
				return Invalid("providerWindow requires counter selectors with exact label matchers")
			}
			if _, err := promqlvalidate.InstantVector(q.text); err != nil {
				return Invalid("%s: %v", q.field, err)
			}
		}
	}
	if s.MeasurementBinding != nil {
		b := s.MeasurementBinding
		if !identifier.MatchString(b.Name) || b.Revision < 1 || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(b.Digest) {
			return Invalid("invalid measurementBinding reference")
		}
	}
	if s.Source != "" && !identifier.MatchString(s.Source) {
		return Invalid("source must be a configured connection name")
	}
	if !identifier.MatchString(s.ID) {
		return Invalid("id must contain 1–63 lowercase letters, digits or hyphens")
	}
	for label, value := range map[string]string{"project": s.Project, "service": s.Service, "name": s.Name} {
		if strings.TrimSpace(value) == "" || len(value) > 200 {
			return Invalid("%s is required and must be at most 200 bytes", label)
		}
	}
	if !Finite(s.Target) || s.Target <= 0 || s.Target >= 1 {
		return Invalid("target must be between 0 and 1, exclusively")
	}
	if s.StepSeconds < 1 || s.StepSeconds > 3600 {
		return Invalid("stepSeconds must be between 1 and 3600")
	}
	if s.WindowSeconds < s.StepSeconds || s.WindowSeconds > 31*86400 || s.WindowSeconds%s.StepSeconds != 0 {
		return Invalid("windowSeconds must be a multiple of stepSeconds, at most 31 days")
	}
	if s.WindowSeconds/s.StepSeconds > 100000 {
		return Invalid("window contains %d intervals at stepSeconds=%d (maximum 100000); shorten windowSeconds or increase the measurement interval", s.WindowSeconds/s.StepSeconds, s.StepSeconds)
	}
	if (s.GoodQuery == "") != (s.TotalQuery == "") || len(s.GoodQuery) > 16000 || len(s.TotalQuery) > 16000 {
		return Invalid("provide both goodQuery and totalQuery, each at most 16000 bytes")
	}
	for _, q := range []struct{ field, text string }{{"goodQuery", s.GoodQuery}, {"totalQuery", s.TotalQuery}} {
		if q.text != "" {
			if _, err := promqlvalidate.InstantVector(q.text); err != nil {
				return Invalid("%s: %v", q.field, err)
			}
		}
	}
	if len(s.Policies) > 8 || (s.Policy != nil && len(s.Policies) > 0) {
		return Invalid("use either policy or up to eight named policies")
	}
	seenPolicies := map[string]bool{}
	for _, p := range s.AlertPolicies() {
		if len(s.Policies) > 0 && (!identifier.MatchString(p.Name) || seenPolicies[p.Name] || (p.Severity != "critical" && p.Severity != "warning" && p.Severity != "info")) {
			return Invalid("policies require unique names and critical, warning or info severity")
		}
		seenPolicies[p.Name] = true
		if p.ShortWindowSeconds < s.StepSeconds || p.LongWindowSeconds < p.ShortWindowSeconds || p.LongWindowSeconds > s.WindowSeconds || p.ShortWindowSeconds%s.StepSeconds != 0 || p.LongWindowSeconds%s.StepSeconds != 0 {
			return Invalid("alert windows must be aligned multiples of stepSeconds within the SLO window")
		}
		if !Finite(p.BurnRate) || p.BurnRate <= 0 || !Finite(p.MinCoverage) || p.MinCoverage <= 0 || p.MinCoverage > 1 || p.ForSeconds < 0 || p.ForSeconds > 31*86400 {
			return Invalid("invalid burn-rate, coverage or pending duration")
		}
	}
	return nil
}

// A point contains event counts over (time-step,time], never cumulative counters.
type Point struct {
	Time  time.Time `json:"time"`
	Good  float64   `json:"good"`
	Total float64   `json:"total"`
}

func Finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func (p Point) Validate(step int64) error {
	if p.Time.IsZero() || p.Time.Nanosecond() != 0 || p.Time.Unix()%step != 0 {
		return Invalid("sample timestamps must align to stepSeconds in UTC")
	}
	if !Finite(p.Good) || !Finite(p.Total) || p.Good < 0 || p.Total < 0 || p.Good > p.Total || p.Total > 1e18 || (p.Total > 0 && p.Total < 1e-9) {
		return Invalid("samples require finite 0 <= good <= total, with total either zero or between 1e-9 and 1e18")
	}
	return nil
}

type AlertState struct {
	Policies      map[string]AlertState `json:"policies,omitempty"`
	Policy        string                `json:"policy,omitempty"`
	Severity      string                `json:"severity,omitempty"`
	PolicyDigest  string                `json:"policyDigest,omitempty"`
	Phase         string                `json:"phase"`
	PendingSince  *time.Time            `json:"pendingSince,omitempty"`
	LastEvaluated *time.Time            `json:"lastEvaluated,omitempty"`
}
type Snapshot struct {
	SLOID                   string     `json:"sloId"`
	At                      time.Time  `json:"at"`
	From                    time.Time  `json:"from"`
	Good                    float64    `json:"good"`
	Total                   float64    `json:"total"`
	Bad                     float64    `json:"bad"`
	Coverage                float64    `json:"coverage"`
	ObservedIntervals       int64      `json:"observedIntervals"`
	ExpectedIntervals       int64      `json:"expectedIntervals"`
	Reliability             *float64   `json:"reliability"`
	BudgetAllowed           *float64   `json:"budgetAllowed"`
	BudgetRemaining         *float64   `json:"budgetRemaining"`
	BudgetRemainingFraction *float64   `json:"budgetRemainingFraction"`
	BurnRate                *float64   `json:"burnRate"`
	DataStatus              string     `json:"dataStatus"`
	Alert                   AlertState `json:"alert"`
}
type AlertEvent struct {
	Policy         string    `json:"policy,omitempty"`
	Severity       string    `json:"severity,omitempty"`
	Reason         string    `json:"reason,omitempty"`
	ConfigRevision int64     `json:"configRevision,omitempty"`
	ID             string    `json:"id"`
	SLOID          string    `json:"sloId"`
	Kind           string    `json:"kind"`
	Time           time.Time `json:"time"`
	ShortBurn      float64   `json:"shortBurn"`
	LongBurn       float64   `json:"longBurn"`
}
type Delivery struct {
	Event       AlertEvent `json:"event"`
	Status      string     `json:"status"`
	Attempts    int        `json:"attempts"`
	NextAttempt time.Time  `json:"nextAttempt"`
	LastError   string     `json:"lastError,omitempty"`
}
type Record struct {
	OperatorRoute string // Pinned before a reviewed operator publication; not user-authored config.
	Windows       map[string]WindowObservation
	Collection    v1alpha1.CollectionHealth

	SLO    SLO
	Points map[int64]Point
	Alert  AlertState
	Events []AlertEvent // New events produced by this transaction only.
}

func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func ConfigRevision(s SLO) int64 {
	if s.Revision < 1 {
		return 1
	}
	return s.Revision
}
