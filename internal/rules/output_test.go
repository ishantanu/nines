package rules

import (
	"testing"

	"github.com/ishantanu/nines/internal/model"
)

func TestOutputEvidenceIdentityTracksDefinitionRevisionAndOwner(t *testing.T) {
	slo := fixture()
	slo.Revision = 4
	group, err := GenerateOwned(slo, "commerce", "resource-a")
	if err != nil {
		t.Fatal(err)
	}
	var marker Rule
	count := 0
	for _, rule := range group.Rules {
		if rule.Record == model.RuleOutputMetric {
			marker = rule
			count++
		}
	}
	if count != 1 || marker.Expr != "vector(1)" || marker.Labels["nines_revision"] != "4" || marker.Labels["nines_resource_uid"] != "resource-a" {
		t.Fatal("missing bounded configuration identity", marker)
	}
	identity := OutputIdentity(slo, "commerce", "resource-a")
	if marker.Labels["nines_definition_digest"] != identity.DefinitionDigest {
		t.Fatal("compiler and query probe use different definition identities")
	}
	slo.Target = .99
	if OutputIdentity(slo, "commerce", "resource-a").DefinitionDigest == identity.DefinitionDigest {
		t.Fatal("target edit retained old configuration marker")
	}
	slo.Target = .999
	slo.Revision = 5
	next := OutputIdentity(slo, "commerce", "resource-b")
	if next.DefinitionDigest != identity.DefinitionDigest || next.Revision != 5 || next.ResourceUID != "resource-b" {
		t.Fatal("revision/owner must remain separate from manifest digest", next)
	}
}
