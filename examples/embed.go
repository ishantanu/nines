package examples

import _ "embed"

//go:embed checkout.journey.yaml
var Checkout string

//go:embed agent-resolution.journey.yaml
var AgentResolution string

//go:embed api-availability.slo.yaml
var APIAvailability string
