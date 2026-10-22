package intentflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
)

type progressModel struct {
	turn func(context.Context, func(rm.Progress) error) (rm.Response, error)
}

func (progressModel) Descriptor() rm.Descriptor {
	return rm.Descriptor{Provider: "fixture", Model: "streaming", RouteDigest: "fixture-stream", Local: true}
}
func (m progressModel) Turn(ctx context.Context, _ rm.Request, progress func(rm.Progress) error) (rm.Response, error) {
	return m.turn(ctx, progress)
}

func TestStreamingProgressIsDurablePrivateAndDoesNotChangeReviewedVersion(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, dsn := fixture(t, pg)
			value, err := s.Create(ctx, input())
			if err != nil {
				t.Fatal(err)
			}
			s.Model = progressModel{turn: func(_ context.Context, progress func(rm.Progress) error) (rm.Response, error) {
				if err := progress(rm.Progress{Bytes: 128}); err != nil {
					t.Fatal(err)
				}
				before, err := s.Read(ctx, value.ID)
				if err != nil {
					t.Fatal(err)
				}
				if before.Runs[0].Progress == nil || before.Runs[0].Progress.Bytes != 128 || before.Runs[0].Progress.State != "receiving" {
					t.Fatal("stream progress not visible", before.Runs)
				}
				if err := progress(rm.Progress{Bytes: 256, Final: true}); err != nil {
					t.Fatal(err)
				}
				reader := s
				if pg {
					repo, err := NewPostgres(ctx, dsn)
					if err != nil {
						t.Fatal(err)
					}
					defer repo.Close()
					reader = New(s.Platform, repo)
				}
				after, err := reader.Read(ctx, value.ID)
				if err != nil {
					t.Fatal(err)
				}
				if after.Version != before.Version || after.Runs[0].Progress.State != "received" || after.Runs[0].Progress.Bytes != 256 || after.Runs[0].Progress.ModelCall != 1 {
					t.Fatal("progress lost or review invalidated", after)
				}
				response := response("ask_user", map[string]string{"question": "Which user outcome should be measured?"})
				response.Native = json.RawMessage(`{"private":"private-provider-native-state"}`)
				return response, nil
			}}
			value, err = s.Start(ctx, value.ID, v1.CreateIntentFlowRun{ID: "stream", ExpectedVersion: value.Version, Mode: "model", ModelConsent: true})
			if err != nil {
				t.Fatal(err)
			}
			work(t, s, ctx, 2)
			value, err = s.Read(ctx, value.ID)
			if err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(value)
			if value.State != "needs_input" || strings.Contains(string(data), "private-provider-native-state") {
				t.Fatal("private state exposed or clarification lost", value.State)
			}
		})
	}
}

func TestCancellationFencesLateStreamingProgress(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	value, err := s.Create(ctx, input())
	if err != nil {
		t.Fatal(err)
	}
	s.Model = progressModel{turn: func(_ context.Context, progress func(rm.Progress) error) (rm.Response, error) {
		if err := progress(rm.Progress{Bytes: 10}); err != nil {
			t.Fatal(err)
		}
		current, err := s.Read(ctx, value.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Cancel(ctx, value.ID, current.Version); err != nil {
			t.Fatal(err)
		}
		if err := progress(rm.Progress{Bytes: 20, Final: true}); !errors.Is(err, errLease) {
			t.Fatal("late progress was accepted", err)
		}
		return rm.Response{}, rm.Failure("model_unavailable")
	}}
	value, err = s.Start(ctx, value.ID, v1.CreateIntentFlowRun{ID: "stream", ExpectedVersion: value.Version, Mode: "model", ModelConsent: true})
	if err != nil {
		t.Fatal(err)
	}
	work(t, s, ctx, 1)
	value, err = s.Read(ctx, value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if value.State != "cancelled" || value.Runs[0].Progress.Bytes != 10 {
		t.Fatal("late worker overwrote cancellation", value)
	}
}
