// Package v1alpha1 defines the optional Kubernetes representation. Domain specs
// and API version are shared with the platform; project is a metadata annotation
// because Kubernetes owns the ObjectMeta schema.
package v1alpha1

import (
	"encoding/json"
	"time"

	domain "github.com/ishantanu/nines/api/v1alpha1"
)

const ProjectAnnotation = "nines.dev/project"
const Finalizer = "nines.dev/ruler-cleanup"

type OwnerReference struct {
	BlockOwnerDeletion *bool  `json:"blockOwnerDeletion,omitempty"`
	APIVersion         string `json:"apiVersion"`
	Kind               string `json:"kind"`
	Name               string `json:"name"`
	UID                string `json:"uid"`
	Controller         bool   `json:"controller"`
}
type Metadata struct {
	CreationTimestamp          *time.Time        `json:"creationTimestamp,omitempty"`
	GenerateName               string            `json:"generateName,omitempty"`
	ManagedFields              []json.RawMessage `json:"managedFields,omitempty"`
	DeletionGracePeriodSeconds *int64            `json:"deletionGracePeriodSeconds,omitempty"`
	Name                       string            `json:"name"`
	Namespace                  string            `json:"namespace,omitempty"`
	UID                        string            `json:"uid,omitempty"`
	ResourceVersion            string            `json:"resourceVersion,omitempty"`
	Generation                 int64             `json:"generation,omitempty"`
	Annotations                map[string]string `json:"annotations,omitempty"`
	Labels                     map[string]string `json:"labels,omitempty"`
	OwnerReferences            []OwnerReference  `json:"ownerReferences,omitempty"`
	Finalizers                 []string          `json:"finalizers,omitempty"`
	DeletionTimestamp          *time.Time        `json:"deletionTimestamp,omitempty"`
}
type Resource struct {
	APIVersion string          `json:"apiVersion"`
	Kind       string          `json:"kind"`
	Metadata   Metadata        `json:"metadata"`
	Spec       json.RawMessage `json:"spec"`
	Status     *Status         `json:"status,omitempty"`
}
type IntentSpec struct {
	domain.IntentSpec
	MeasurementSelection MeasurementSelection `json:"measurementSelection"`
}
type Condition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	Reason             string    `json:"reason"`
	Message            string    `json:"message"`
	ObservedGeneration int64     `json:"observedGeneration"`
	LastTransitionTime time.Time `json:"lastTransitionTime"`
}
type Status struct {
	MeasurementDigest  string      `json:"measurementDigest,omitempty"`
	InputDigest        string      `json:"inputDigest,omitempty"`
	ObservedGeneration int64       `json:"observedGeneration"`
	Conditions         []Condition `json:"conditions"`
	DefinitionDigest   string      `json:"definitionDigest,omitempty"`
	RuleDigest         string      `json:"ruleDigest,omitempty"`
	DestinationDigest  string      `json:"destinationDigest,omitempty"`
	Source             string      `json:"source,omitempty"`
	ManagedGroup       string      `json:"managedGroup,omitempty"`
	BindingGeneration  int64       `json:"bindingGeneration,omitempty"`
	ValidatedAt        *time.Time  `json:"validatedAt,omitempty"`
}

type MeasurementSelection struct {
	Name               string `json:"name"`
	ExpectedGeneration int64  `json:"expectedGeneration"`
}
