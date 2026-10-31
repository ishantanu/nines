// Package provider is the compiled Go telemetry extension API for Nines.
// Adapters register during host startup. No SLO, agent or tenant configuration
// can install code. This contract is experimental and versioned independently.
package provider

import (
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/providers"
	"github.com/ishantanu/nines/internal/queryspec"
	"github.com/ishantanu/nines/internal/telemetryprovider"
)

const APIVersion = "nines.dev/provider/v1alpha1"

type SLO = model.SLO
type Point = model.Point
type Collector = telemetryprovider.Collector
type Configuration = telemetryprovider.Configuration
type Capabilities = telemetryprovider.Capabilities
type Descriptor = telemetryprovider.Descriptor
type Factory = telemetryprovider.Factory
type Registry = telemetryprovider.Registry
type QueryValidator = queryspec.Validator

func NewRegistry() *Registry { return telemetryprovider.NewRegistry() }

// Register installs a compiled adapter in the host registry. Duplicate kinds
// are rejected. Complete registrations before serving requests.
func Register(descriptor Descriptor, factory Factory) error {
	return providers.Default.Register(descriptor, factory)
}
func RegisterQueryLanguage(name string, validate QueryValidator) error {
	return queryspec.RegisterLanguage(name, validate)
}

type WindowObservation = model.WindowObservation
type WindowCollector = telemetryprovider.WindowCollector
type WindowPreviewer = telemetryprovider.WindowPreviewer
type MetricNamesSource = telemetryprovider.MetricNamesSource
type MetricDiscoverySource = telemetryprovider.MetricDiscoverySource
type DependencySource = telemetryprovider.DependencySource
type CohortSource = telemetryprovider.CohortSource
type Prober = telemetryprovider.Prober
type RuleSupport = telemetryprovider.RuleSupport

type MeasurementLimitations = telemetryprovider.MeasurementLimitations
