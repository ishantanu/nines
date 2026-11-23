package investigation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/postmortem"
	"github.com/ishantanu/nines/internal/tenancy"
)

var dueDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// latestPostmortem is the newest postmortem revision on a review.
func latestPostmortem(v v1.Investigation) (*v1.Postmortem, string) {
	for n := len(v.Entries) - 1; n >= 0; n-- {
		if e := v.Entries[n]; e.Kind == "postmortem" && e.Postmortem != nil {
			return e.Postmortem, e.ID
		}
	}
	return nil, ""
}

// incidentFor reads the incident an incident review is about.
func (s *Service) incidentFor(ctx context.Context, v v1.Investigation) (bool, error) {
	if v.Scope.Kind != v1.IncidentScope {
		return false, model.Invalid("postmortems are written on reviews of an incident; start one from the incident")
	}
	if !s.Platform.IncidentsEnabled {
		return false, model.Invalid("incident reviews need the incidents feature")
	}
	incident, err := s.Platform.Incident(ctx, v.Scope.ResourceID)
	if err != nil {
		return false, err
	}
	return incident.DetectedBySLO, nil
}

// PostmortemTemplate returns the latest revision, or a new postmortem. An
// incident no SLO detected starts with the standing action item to define
// one, so the gap becomes tracked work.
func (s *Service) PostmortemTemplate(ctx context.Context, id string) (v1.PostmortemTemplate, error) {
	v, err := s.Read(ctx, id)
	if err != nil {
		return v1.PostmortemTemplate{}, err
	}
	detected, err := s.incidentFor(ctx, v)
	if err != nil {
		return v1.PostmortemTemplate{}, err
	}
	if latest, revision := latestPostmortem(v); latest != nil {
		return v1.PostmortemTemplate{Postmortem: *latest, Revision: revision, Findings: Findings(*latest)}, nil
	}
	p := v1.Postmortem{AddedEvents: []v1.PostmortemEvent{}, ContributingCauses: []v1.ContributingCause{}, Lessons: v1.PostmortemLessons{WentWell: []string{}, WentPoorly: []string{}, GotLucky: []string{}}, ActionItems: []v1.ActionItem{}}
	if !detected {
		p.ActionItems = append(p.ActionItems, standingItem())
	}
	return v1.PostmortemTemplate{Postmortem: p, Findings: []v1.HindsightFinding{}}, nil
}

func standingItem() v1.ActionItem {
	return v1.ActionItem{ID: v1.DefineSLOAction, Title: "Define an SLO that would have caught this incident", Kind: v1.ActionDetect}
}

// Findings flags hindsight and blame wording in every written field.
func Findings(p v1.Postmortem) []v1.HindsightFinding {
	out := []v1.HindsightFinding{}
	add := func(field, text string) {
		for _, f := range postmortem.Check(text) {
			out = append(out, v1.HindsightFinding{Field: field, Phrase: f.Phrase, Suggestion: f.Suggestion})
		}
	}
	add("summary", p.Summary)
	for n, e := range p.AddedEvents {
		add(fmt.Sprintf("addedEvents[%d]", n), e.Text)
	}
	for n, c := range p.ContributingCauses {
		add(fmt.Sprintf("contributingCauses[%d]", n), c.Text)
	}
	for name, list := range map[string][]string{"lessons.wentWell": p.Lessons.WentWell, "lessons.wentPoorly": p.Lessons.WentPoorly, "lessons.gotLucky": p.Lessons.GotLucky} {
		for n, text := range list {
			add(fmt.Sprintf("%s[%d]", name, n), text)
		}
	}
	for n, a := range p.ActionItems {
		add(fmt.Sprintf("actionItems[%d]", n), a.Title)
	}
	return out
}

func boundedText(text string, max int) bool {
	return utf8.ValidString(text) && len(strings.TrimSpace(text)) > 0 && len(text) <= max
}

func (s *Service) validatePostmortem(ctx context.Context, p v1.Postmortem, detected bool) error {
	if len(p.Summary) > 8000 || !utf8.ValidString(p.Summary) {
		return model.Invalid("summary must be at most 8000 bytes of UTF-8 text")
	}
	if len(p.AddedEvents) > 100 || len(p.ContributingCauses) > 50 || len(p.ActionItems) > 50 || len(p.Lessons.WentWell)+len(p.Lessons.WentPoorly)+len(p.Lessons.GotLucky) > 60 {
		return model.Invalid("at most 100 added events, 50 contributing causes, 50 action items and 60 lessons")
	}
	for _, e := range p.AddedEvents {
		if e.Time.IsZero() || !boundedText(e.Text, 1000) {
			return model.Invalid("each added event needs a time and 1 to 1000 bytes of text")
		}
	}
	for _, c := range p.ContributingCauses {
		if c.Kind != v1.CauseTrigger && c.Kind != v1.CauseLatent && c.Kind != v1.CauseSafeguard {
			return model.Invalid("a contributing cause is a trigger, a latent condition or a safeguard")
		}
		if !boundedText(c.Text, 2000) {
			return model.Invalid("each contributing cause needs 1 to 2000 bytes of text")
		}
	}
	for _, list := range [][]string{p.Lessons.WentWell, p.Lessons.WentPoorly, p.Lessons.GotLucky} {
		for _, text := range list {
			if !boundedText(text, 1000) {
				return model.Invalid("each lesson needs 1 to 1000 bytes of text")
			}
		}
	}
	seen := map[string]bool{}
	standing := false
	for _, a := range p.ActionItems {
		if !identifier.MatchString(a.ID) || seen[a.ID] {
			return model.Invalid("each action item needs a unique ID")
		}
		seen[a.ID] = true
		standing = standing || a.ID == v1.DefineSLOAction
		if !boundedText(a.Title, 300) || len(a.Owner) > 200 {
			return model.Invalid("each action item needs a title of at most 300 bytes and an owner of at most 200")
		}
		if a.Kind != v1.ActionPrevent && a.Kind != v1.ActionDetect && a.Kind != v1.ActionMitigate {
			return model.Invalid("an action item prevents, detects or mitigates")
		}
		if a.Due != "" {
			if _, err := time.Parse("2006-01-02", a.Due); err != nil || !dueDate.MatchString(a.Due) {
				return model.Invalid("due dates are YYYY-MM-DD")
			}
		}
		if len(a.SLOs) > 20 || len(a.Journeys) > 20 {
			return model.Invalid("an action item links at most 20 SLOs and 20 journeys")
		}
		for _, id := range a.SLOs {
			if _, err := s.Platform.Store.Read(ctx, id); err != nil {
				return model.Invalid("action item %s links unknown SLO %s", a.ID, id)
			}
		}
		for _, id := range a.Journeys {
			if s.Platform.Journeys == nil {
				return model.Invalid("journeys are not available")
			}
			if _, err := s.Platform.Journeys.JourneyRevision(ctx, id, 0); err != nil {
				return model.Invalid("action item %s links unknown journey %s", a.ID, id)
			}
		}
	}
	if !detected && !standing {
		return model.Invalid("keep the action item %q: no SLO detected this incident. Mark it done once the SLO exists", standingItem().Title)
	}
	return nil
}

// SavePostmortem appends a postmortem revision. People write postmortems,
// so it needs the owner or editor role; agents may help in messages.
func (s *Service) SavePostmortem(ctx context.Context, id string, in v1.SavePostmortem) (v1.Investigation, []v1.HindsightFinding, error) {
	actor, err := s.actor(ctx)
	if err != nil {
		return v1.Investigation{}, nil, err
	}
	if p, _ := tenancy.FromContext(ctx); p.Role != "owner" && p.Role != "editor" {
		return v1.Investigation{}, nil, model.Invalid("people write postmortems: it needs the owner or editor role")
	}
	if !identifier.MatchString(in.ID) || in.ExpectedVersion < 1 {
		return v1.Investigation{}, nil, model.Invalid("saving needs a revision ID and the review's expectedVersion")
	}
	v, err := s.Read(ctx, id)
	if err != nil {
		return v, nil, err
	}
	detected, err := s.incidentFor(ctx, v)
	if err != nil {
		return v, nil, err
	}
	p := in.Postmortem
	if p.AddedEvents == nil {
		p.AddedEvents = []v1.PostmortemEvent{}
	}
	if p.ContributingCauses == nil {
		p.ContributingCauses = []v1.ContributingCause{}
	}
	if p.ActionItems == nil {
		p.ActionItems = []v1.ActionItem{}
	}
	for n := range p.AddedEvents {
		p.AddedEvents[n].Time = p.AddedEvents[n].Time.UTC().Truncate(time.Second)
	}
	if err = s.validatePostmortem(ctx, p, detected); err != nil {
		return v, nil, err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return v, nil, err
	}
	if len(b) > 64<<10 {
		return v, nil, model.Invalid("a postmortem is at most 64 KiB")
	}
	sum := sha256.Sum256(b)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	saved, err := s.append(ctx, id, in.ExpectedVersion, v1.InvestigationEntry{ID: in.ID, Kind: "postmortem", Actor: actor, CreatedAt: s.Platform.Now().UTC(), Text: digest, Postmortem: &p})
	if err != nil {
		return saved, nil, err
	}
	// Record it on the incident, so a required postmortem counts as written
	// once published. A retried save is a no-op above and repeats this.
	if err = s.Platform.RecordPostmortem(ctx, v.Scope.ResourceID, v.ID, p.Published); err != nil {
		return saved, nil, fmt.Errorf("postmortem saved, but linking it to the incident failed; save again: %w", err)
	}
	return saved, Findings(p), nil
}

// OpenActionItems lists open action items from the latest postmortem of each
// incident review, optionally only those linking an SLO or a journey.
func (s *Service) OpenActionItems(ctx context.Context, slo, journey string) ([]v1.OpenActionItem, error) {
	out := []v1.OpenActionItem{}
	after := ""
	for page := 0; page < 20; page++ {
		rows, err := s.Repository.List(ctx, after, 100)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.Scope.Kind != v1.IncidentScope {
				continue
			}
			v, err := s.Repository.Read(ctx, row.ID)
			if err != nil {
				return nil, err
			}
			p, _ := latestPostmortem(v)
			if p == nil {
				continue
			}
			for _, a := range p.ActionItems {
				if a.Done || slo != "" && !contains(a.SLOs, slo) || journey != "" && !contains(a.Journeys, journey) {
					continue
				}
				out = append(out, v1.OpenActionItem{ReviewID: v.ID, ReviewTitle: v.Title, IncidentID: v.Scope.ResourceID, Item: a})
			}
		}
		if len(rows) < 100 {
			return out, nil
		}
		after = rows[len(rows)-1].ID
	}
	return out, nil
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// IncidentPostmortem summarizes the postmortem on one review of an incident.
type IncidentPostmortem struct {
	ReviewID    string                `json:"reviewId"`
	ReviewTitle string                `json:"reviewTitle"`
	Published   bool                  `json:"published"`
	SavedAt     time.Time             `json:"savedAt"`
	SavedBy     v1.InvestigationActor `json:"savedBy"`
	OpenActions int                   `json:"openActions"`
}

// IncidentPostmortems lists the postmortems written about an incident.
func (s *Service) IncidentPostmortems(ctx context.Context, incident string) ([]IncidentPostmortem, error) {
	if !identifier.MatchString(incident) {
		return nil, model.Invalid("invalid incident ID")
	}
	out := []IncidentPostmortem{}
	after := ""
	for page := 0; page < 20; page++ {
		rows, err := s.Repository.List(ctx, after, 100)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.Scope.Kind != v1.IncidentScope || row.Scope.ResourceID != incident {
				continue
			}
			v, err := s.Repository.Read(ctx, row.ID)
			if err != nil {
				return nil, err
			}
			for n := len(v.Entries) - 1; n >= 0; n-- {
				e := v.Entries[n]
				if e.Kind != "postmortem" || e.Postmortem == nil {
					continue
				}
				open := 0
				for _, a := range e.Postmortem.ActionItems {
					if !a.Done {
						open++
					}
				}
				out = append(out, IncidentPostmortem{ReviewID: v.ID, ReviewTitle: v.Title, Published: e.Postmortem.Published, SavedAt: e.CreatedAt, SavedBy: e.Actor, OpenActions: open})
				break
			}
		}
		if len(rows) < 100 {
			return out, nil
		}
		after = rows[len(rows)-1].ID
	}
	return out, nil
}
