package model

// RuleOutputMetric is a configuration marker, independent of traffic or SLI
// arithmetic. Its labels identify the compiler input and owning resource.
const RuleOutputMetric = "nines:rule_definition:info"

type RuleOutputIdentity struct {
	Tenant, Source, SLO, DefinitionDigest, ResourceUID string
	Revision, StepSeconds                              int64
}
