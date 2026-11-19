package v1alpha1

import "time"

// CLIAuthenticationCapabilities describes the Nines-mediated CLI protocol, not
// an upstream provider's OAuth discovery or a general OAuth authorization server.
type CLIAuthenticationCapabilities struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	Browser           bool   `json:"browser"`
	Device            bool   `json:"device"`
	Refresh           bool   `json:"refresh"`
	APIKey            bool   `json:"apiKey"`
	SessionMaxSeconds int    `json:"sessionMaxSeconds"`
}
type CLIDeviceStart struct {
	Tenant    string `json:"tenant"`
	Challenge string `json:"challenge"`
}
type CLIDeviceAuthorization struct {
	DeviceCode      string    `json:"deviceCode"`
	UserCode        string    `json:"userCode"`
	VerificationURI string    `json:"verificationURI"`
	ExpiresAt       time.Time `json:"expiresAt"`
	IntervalSeconds int       `json:"intervalSeconds"`
}
type CLIDevicePoll struct {
	DeviceCode   string `json:"deviceCode"`
	CodeVerifier string `json:"codeVerifier"`
}
type CLIDeviceResult struct {
	Status          string    `json:"status"`
	IntervalSeconds int       `json:"intervalSeconds,omitempty"`
	Token           string    `json:"token,omitempty"`
	ExpiresAt       time.Time `json:"expiresAt,omitempty"`
	Tenant          string    `json:"tenant,omitempty"`
	Identity        string    `json:"identity,omitempty"`
}
