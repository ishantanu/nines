package model

import (
	"fmt"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/api/v1beta1"
)

// SLOYAML renders a generated SLO for people to review and apply. It is
// v1beta1, checked to parse back to exactly the same definition, so applying
// it stores what was generated. An SLO carrying intent provenance stays
// v1alpha1, the only version that can express it.
func SLOYAML(slo v1.ServiceLevelObjective) ([]byte, error) {
	if slo.Spec.Intent != nil {
		return yaml.Marshal(slo)
	}
	body, err := yaml.Marshal(v1beta1.FromHub(slo))
	if err != nil {
		return nil, err
	}
	back, err := ParseSLODefinition(body)
	if err != nil {
		return nil, fmt.Errorf("generated v1beta1 SLO does not parse: %w", err)
	}
	if DefinitionDigest(derivedFree(v1.ServiceLevelObjective(back))) != DefinitionDigest(derivedFree(slo)) {
		return nil, fmt.Errorf("generated v1beta1 SLO %s does not round-trip", slo.Metadata.Name)
	}
	return body, nil
}

// derivedFree drops what the server derives on apply: an inline measurement's
// window, and a binding reference's queries or window.
func derivedFree(slo v1.ServiceLevelObjective) v1.ServiceLevelObjective {
	if slo.Spec.Measurement != nil {
		slo.Spec.ProviderWindow = nil
	}
	if slo.Spec.MeasurementBinding != nil {
		slo.Spec.GoodQuery, slo.Spec.TotalQuery, slo.Spec.ProviderWindow = "", "", nil
	}
	return slo
}
