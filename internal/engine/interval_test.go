package engine

import (
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// A steady 5% noise floor cannot meet a 99.9% event ratio, but under a
// good-minute definition it costs nothing until a minute actually breaks.
func TestBadIntervalCountsMinutesNotEvents(t *testing.T) {
	plain := config()
	plain.Target = .999
	plain.WindowSeconds = 600
	bad := plain
	bad.BadInterval = &v1.BadIntervalPolicy{ErrorRate: 0.06}
	points := map[int64]model.Point{}
	for i := 0; i < 10; i++ {
		at := epoch.Add(-time.Duration(9-i) * time.Minute)
		p := model.Point{Time: at, Good: 950, Total: 1000} // 5% noise every minute
		switch i {
		case 4:
			p = model.Point{Time: at, Good: 100, Total: 1000} // a real incident minute
		case 6:
			p = model.Point{Time: at} // no traffic: not judged
		}
		points[at.Unix()] = p
	}
	e := Evaluate(plain, points, epoch, 600)
	if e.Reliability == nil || *e.Reliability > 0.9 {
		t.Fatalf("the event ratio should be dragged down by noise: %v", e.Reliability)
	}
	m := Evaluate(bad, points, epoch, 600)
	// Nine judged minutes (one was idle), one of them bad.
	if m.Total != 9 || m.Good != 8 || m.Bad != 1 {
		t.Fatalf("judged minutes = %v good %v bad %v", m.Total, m.Good, m.Bad)
	}
	closeTo(t, m.Reliability, 8.0/9)
	// Burn rate is the bad-minute share over the allowed share.
	closeTo(t, m.BurnRate, (1.0/9)/0.001)
	// Coverage still counts every observed step, including the idle one.
	if m.Coverage != 1 || m.ObservedIntervals != 10 {
		t.Fatalf("coverage must not depend on traffic: %+v", m)
	}
}

func TestBadIntervalWithNothingJudgedStaysUnknown(t *testing.T) {
	s := config()
	s.BadInterval = &v1.BadIntervalPolicy{ErrorRate: 0.06, MinEvents: 100}
	points := map[int64]model.Point{epoch.Unix(): {Time: epoch, Good: 5, Total: 5}}
	r := Evaluate(s, points, epoch, 60)
	if r.Reliability != nil || r.DataStatus != "no_traffic" {
		t.Fatalf("an unjudged interval must not read as perfect: %+v", r)
	}
}

func TestThroughputCountsMinutesThatKeptUp(t *testing.T) {
	s := config()
	s.Target = .9
	s.WindowSeconds = 600
	s.BadInterval = &v1.BadIntervalPolicy{MinGoodEvents: 500}
	points := map[int64]model.Point{}
	for i := 0; i < 10; i++ {
		at := epoch.Add(-time.Duration(9-i) * time.Minute)
		p := model.Point{Time: at, Good: 600, Total: 610}
		switch i {
		case 3:
			p = model.Point{Time: at, Good: 120, Total: 130} // slowed down
		case 4:
			p = model.Point{Time: at} // stalled: observed, nothing processed
		case 7:
			continue // no data at all: unknown, not judged
		}
		points[at.Unix()] = p
	}
	r := Evaluate(s, points, epoch, 600)
	// Nine observed minutes, two of them below the minimum.
	if r.Total != 9 || r.Good != 7 || r.Bad != 2 {
		t.Fatalf("judged minutes = %v good %v bad %v", r.Total, r.Good, r.Bad)
	}
	closeTo(t, r.Reliability, 7.0/9)
	// The missing minute lowers coverage instead of counting as bad.
	if r.ObservedIntervals != 9 || r.Coverage != 0.9 {
		t.Fatalf("a gap must stay unknown: %+v", r)
	}
}
