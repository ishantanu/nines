package intentmodel

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Config is server-owned. It contains credential references, never secret values.
type Config struct {
	Provider   string `json:"provider,omitempty" yaml:"provider,omitempty"`
	Endpoint   string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	Model      string `json:"model,omitempty" yaml:"model,omitempty"`
	APIKeyEnv  string `json:"apiKeyEnv,omitempty" yaml:"apiKeyEnv,omitempty"`
	APIStyle   string `json:"apiStyle,omitempty" yaml:"apiStyle,omitempty"`
	OutputMode string `json:"outputMode,omitempty" yaml:"outputMode,omitempty"`
}
type preset struct {
	provider, style, endpoint, model, output string
	keys                                     []string
}

var presets = []preset{
	{"anthropic", "anthropic", "https://api.anthropic.com/v1/messages", "claude-haiku-5-5", "schema", []string{"ANTHROPIC_API_KEY", "ATHROPIC_API_KEY"}},
	{"gemini", "gemini", "https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent", "gemini-3.5-flash-lite", "schema", []string{"GEMINI_API_KEY", "GOOGLE_API_KEY"}},
	{"mistral", "chat-completions", "https://api.mistral.ai/v1/chat/completions", "mistral-small-latest", "schema", []string{"MISTRAL_API_KEY"}},
	{"groq", "chat-completions", "https://api.groq.com/openai/v1/chat/completions", "llama-3.3-70b-versatile", "json", []string{"GROQ_API_KEY"}},
	{"deepseek", "chat-completions", "https://api.deepseek.com/chat/completions", "deepseek-flash", "json", []string{"DEEPSEEK_API_KEY"}},
	{"openrouter", "chat-completions", "https://openrouter.ai/api/v1/chat/completions", "anthropic/claude-haiku-4.5", "json", []string{"OPENROUTER_API_KEY"}},
	{"xai", "chat-completions", "https://api.x.ai/v1/chat/completions", "grok-4.7", "schema", []string{"XAI_API_KEY"}},
	{"openai", "openai", "https://api.openai.com/v1/chat/completions", "gpt-4.1-mini", "schema", []string{"OPENAI_API_KEY"}},
	{"ollama", "chat-completions", "http://127.0.0.1:11434/v1/chat/completions", "", "schema", nil},
	{"vllm", "chat-completions", "http://127.0.0.1:8000/v1/chat/completions", "", "schema", nil},
	{"custom", "chat-completions", "", "", "schema", nil},
}
var envName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
var nativeModelID = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func providerPreset(provider string) (preset, bool) {
	if provider == "google" {
		provider = "gemini"
	}
	if provider == "openai-compatible" || provider == "chat-completions" {
		provider = "custom"
	}
	for _, p := range presets {
		if p.provider == provider {
			return p, true
		}
	}
	return preset{}, false
}
func presetKey(p preset, lookup func(string) string) (string, error) {
	key := ""
	for _, name := range p.keys {
		v := lookup(name)
		if v == "" {
			continue
		}
		if key != "" && key != v {
			return "", fmt.Errorf("conflicting credential aliases for provider %s; use apiKeyEnv or INTENT_MODEL_API_KEY_ENV", p.provider)
		}
		key = v
	}
	return key, nil
}

// FromEnvironment resolves explicit tenant configuration or an environment default.
// It never probes a provider, retries elsewhere, or reads unrelated environment values.
// A nil client means disabled. An explicit tenant config never inherits global routing.
func FromEnvironment(in *Config, lookup func(string) string) (*Client, error) {
	var c Config
	global := in == nil
	if global {
		c = Config{Provider: lookup("INTENT_MODEL_PROVIDER"), Model: lookup("INTENT_MODEL_MODEL"), Endpoint: lookup("INTENT_MODEL_ENDPOINT"), APIKeyEnv: lookup("INTENT_MODEL_API_KEY_ENV"), APIStyle: lookup("INTENT_MODEL_API_STYLE"), OutputMode: lookup("INTENT_MODEL_OUTPUT_MODE")}
	} else {
		c = *in
	}
	c.Provider = strings.ToLower(strings.TrimSpace(c.Provider))
	if c.Provider == "none" {
		return nil, nil
	}
	// An explicit endpoint without a provider preserves the old portable adapter.
	if c.Provider == "" && c.Endpoint != "" && !global {
		c.Provider = "custom"
	}
	if c.Provider == "" && c.APIKeyEnv != "" {
		for _, p := range presets {
			for _, k := range p.keys {
				if k == c.APIKeyEnv {
					c.Provider = p.provider
				}
			}
		}
	}
	if c.Provider == "" && global {
		for _, p := range presets {
			key, err := presetKey(p, lookup)
			if err != nil {
				return nil, err
			}
			if key != "" {
				if c.Provider != "" {
					return nil, errors.New("multiple LLM provider keys configured; set INTENT_MODEL_PROVIDER or a tenant intentModel.provider")
				}
				c.Provider = p.provider
			}
		}
		if c.Provider == "" && c.Model == "" && c.APIStyle == "" && c.APIKeyEnv == "" && c.OutputMode == "" && c.Endpoint == "" {
			return nil, nil
		}
	}
	if c.Provider == "" && c.Endpoint != "" {
		c.Provider = "custom"
	}
	if c.Provider == "" {
		return nil, errors.New("configure an intent model provider or a custom endpoint")
	}
	p, ok := providerPreset(c.Provider)
	if !ok {
		return nil, errors.New("unsupported intent model provider; configure a supported provider or custom with endpoint and apiStyle")
	}
	c.Provider = p.provider
	key := ""
	if c.APIKeyEnv != "" {
		if !envName.MatchString(c.APIKeyEnv) {
			return nil, errors.New("invalid intent model credential environment reference")
		}
		key = lookup(c.APIKeyEnv)
		if key == "" {
			return nil, fmt.Errorf("required environment variable %s is empty", c.APIKeyEnv)
		}
	} else {
		var err error
		key, err = presetKey(p, lookup)
		if err != nil {
			return nil, err
		}
	}
	if len(p.keys) > 0 && key == "" {
		return nil, fmt.Errorf("provider %s needs its API key environment variable or apiKeyEnv", p.provider)
	}
	return NewConfigured(c, key)
}

// NewConfigured supports native provider protocols without any provider SDK.
func NewConfigured(c Config, key string) (*Client, error) {
	provider := strings.ToLower(strings.TrimSpace(c.Provider))
	if provider == "" {
		provider = "custom"
	}
	p, ok := providerPreset(provider)
	if !ok {
		return nil, errors.New("unsupported intent model provider")
	}
	if c.Endpoint == "" {
		c.Endpoint = p.endpoint
	}
	if c.Model == "" {
		c.Model = p.model
	}
	if c.APIStyle == "" {
		c.APIStyle = p.style
	}
	if c.OutputMode == "" {
		c.OutputMode = p.output
	}
	if c.APIStyle == "openai-compatible" {
		c.APIStyle = "chat-completions"
	}
	if p.provider == "anthropic" && c.APIStyle != "anthropic" || p.provider == "gemini" && c.APIStyle != "gemini" {
		return nil, errors.New("native provider and apiStyle disagree")
	}
	if strings.Contains(c.Endpoint, "{model}") {
		if !nativeModelID.MatchString(c.Model) {
			return nil, errors.New("invalid native model identifier")
		}
		c.Endpoint = strings.ReplaceAll(c.Endpoint, "{model}", c.Model)
	}
	client, err := newClient(c.Endpoint, c.Model, key, c.APIStyle, c.OutputMode)
	if err != nil {
		return nil, err
	}
	client.provider = p.provider
	return client, nil
}
