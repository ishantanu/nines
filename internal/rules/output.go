package rules

import (
	"strconv"

	"github.com/ishantanu/nines/internal/model"
)

func OutputIdentity(s model.SLO, tenant, uid string) model.RuleOutputIdentity {
	return model.RuleOutputIdentity{Tenant: tenant, Source: ConnectionID(s), SLO: s.ID, DefinitionDigest: model.DefinitionDigest(model.DefinitionFromSLO(s)), Revision: model.ConfigRevision(s), ResourceUID: uid, StepSeconds: s.StepSeconds}
}

func withOutputMarker(group Group, s model.SLO, tenant, uid string) Group {
	identity := OutputIdentity(s, tenant, uid)
	labels := map[string]string{"nines_managed": "true", "nines_tenant": tenant, "nines_source": identity.Source, "nines_slo": s.ID, "nines_revision": strconv.FormatInt(identity.Revision, 10), "nines_definition_digest": identity.DefinitionDigest}
	if s.ProviderWindow != nil {
		labels["nines_measurement"] = "provider_window"
	}
	if uid != "" {
		labels["nines_resource_uid"] = uid
	}
	marker := Rule{Record: model.RuleOutputMetric, Expr: "vector(1)", Labels: labels}
	index := len(group.Rules)
	for n, rule := range group.Rules {
		if rule.Alert != "" {
			index = n
			break
		}
	}
	next := make([]Rule, 0, len(group.Rules)+1)
	next = append(next, group.Rules[:index]...)
	next = append(next, marker)
	group.Rules = append(next, group.Rules[index:]...)
	return group
}
