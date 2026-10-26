package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/slotemplates"
)

func TestOfflineExtensionInstallGenerateAndOperatorParity(t *testing.T) {
	source := filepath.Join("..", "..", "examples", "extensions", "example-agent.yaml")
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	lock := filepath.Join(root, "extensions.lock.yaml")
	values := filepath.Join(root, "values.yaml")
	body := []byte("name: support-evaluation\nsource: primary\ninputs:\n  metric: support_evaluations_total\n  service: support-agent\n  population:\n    service: support-agent\n    rubric: support-v1\n    evaluator: rules-v1\n")
	if err = os.WriteFile(values, body, 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	for _, args := range [][]string{{"--offline", "extensions", "test", "-f", source}, {"--offline", "extensions", "install", "-f", source, "--sha256", slotemplates.SHA256(data), "--lock", lock}, {"--offline", "extensions", "list", "--lock", lock}} {
		out.Reset()
		if err = run(args, &out); err != nil {
			t.Fatal(args, err)
		}
		if !json.Valid(out.Bytes()) {
			t.Fatal(out.String())
		}
	}
	out.Reset()
	if err = run([]string{"--offline", "templates", "generate", "example-agent--evaluation-pass", "--extension-lock", lock, "--values", values}, &out); err != nil {
		t.Fatal(err)
	}
	slo, err := model.ParseSLODefinition(out.Bytes())
	if err != nil || slo.Metadata.Name != "support-evaluation" || slo.Spec.Service != "support-agent" {
		t.Fatal(out.String(), err)
	}
	out.Reset()
	if err = run([]string{"--offline", "operator", "render", "--template", "example-agent--evaluation-pass", "--extension-lock", lock, "--values", values, "--namespace", "team-support"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "kind: ServiceLevelObjective") || strings.Contains(out.String(), "extension-lock") || !strings.Contains(out.String(), "support_evaluations_total") {
		t.Fatal(out.String())
	}
	out.Reset()
	if err = run([]string{"--offline", "operator", "render", "--template", "example-agent--evaluation-pass", "--extension-lock", lock, "--values", values, "--namespace", "team-support", "--tenant", "support", "--rules-only"}, &out); err != nil || !strings.Contains(out.String(), "kind: PrometheusRule") {
		t.Fatal(err, out.String())
	}
	if err = run([]string{"--offline", "extensions", "install", "--url", "https://example.invalid/extension.yaml", "--sha256", slotemplates.SHA256(data), "--lock", lock}, &out); err == nil {
		t.Fatal("offline download accepted")
	}
}

func TestOfflineGitExtensionCLI(t *testing.T) {
	repository := t.TempDir()
	fixture := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "user.name=Nines Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=" + filepath.Join(repository, "no-hooks")}, args...)...)
		command.Dir = repository
		command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + repository, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull}
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("Git fixture: %v: %s", err, output)
		}
		return strings.TrimSpace(string(output))
	}
	fixture("init", "--quiet", ".")
	data, err := os.ReadFile("../../examples/extensions/example-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(repository, "bundle.yaml"), data, 0600); err != nil {
		t.Fatal(err)
	}
	fixture("add", "bundle.yaml")
	fixture("commit", "--quiet", "-m", "Extension fixture")
	commit := fixture("rev-parse", "HEAD")
	lock := filepath.Join(t.TempDir(), "extensions.lock.yaml")
	args := []string{"--offline", "--agent", "extensions", "install", "--git", repository, "--commit", commit, "--git-path", "bundle.yaml", "--sha256", slotemplates.SHA256(data), "--lock", lock}
	var out bytes.Buffer
	if err = run(args, &out); err != nil {
		t.Fatal(err, out.String())
	}
	var receipt struct {
		Kind        string                       `json:"kind"`
		NetworkUsed bool                         `json:"networkUsed"`
		Git         slotemplates.GitBundleSource `json:"git"`
	}
	if err = json.Unmarshal(out.Bytes(), &receipt); err != nil || receipt.Kind != "SLOExtensionInstallation" || receipt.NetworkUsed || receipt.Git.Commit != commit || receipt.Git.Path != "bundle.yaml" {
		t.Fatal(err, out.String())
	}
	out.Reset()
	if err = run([]string{"--offline", "templates", "show", "example-agent--evaluation-pass", "--extension-lock", lock}, &out); err != nil {
		t.Fatal(err, out.String())
	}
	for name, invalid := range map[string][]string{
		"missing commit/path": {"--git", repository},
		"floating ref":        {"--git", repository, "--commit", "main", "--git-path", "bundle.yaml"},
		"remote offline":      {"--git", "https://example.invalid/repo.git", "--commit", commit, "--git-path", "bundle.yaml"},
		"mixed sources":       {"--git", repository, "--commit", commit, "--git-path", "bundle.yaml", "--file", "bundle.yaml"},
		"orphan commit":       {"--file", filepath.Join(repository, "bundle.yaml"), "--commit", commit},
	} {
		t.Run(name, func(t *testing.T) {
			out.Reset()
			argv := append([]string{"--offline", "--agent", "extensions", "install", "--lock", lock, "--sha256", slotemplates.SHA256(data)}, invalid...)
			if err := run(argv, &out); err == nil {
				t.Fatal("invalid invocation accepted", out.String())
			}
		})
	}
}
