package slotemplates

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "user.name=Nines Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=" + filepath.Join(directory, "no-hooks")}, args...)...)
	command.Dir = directory
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + directory, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git fixture failed: %v: %s", err, output)
	}
	return strings.TrimSpace(string(output))
}

func gitBundleFixture(t *testing.T, format string) (GitBundleSource, []byte) {
	t.Helper()
	repository := t.TempDir()
	fixtureGit(t, repository, "init", "--quiet", "--object-format="+format, ".")
	data := exampleExtension(t)
	if err := os.Mkdir(filepath.Join(repository, "bundles"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "bundles", "agent.yaml"), data, 0600); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, repository, "add", "bundles/agent.yaml")
	fixtureGit(t, repository, "commit", "--quiet", "-m", "Fixture bundle")
	return GitBundleSource{Repository: repository, Commit: fixtureGit(t, repository, "rev-parse", "HEAD"), Path: "bundles/agent.yaml"}, data
}

func TestGitBundlePinnedCommitIndependentOfWorkingTreeAndSource(t *testing.T) {
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) {
			source, data := gitBundleFixture(t, format)
			if err := os.WriteFile(filepath.Join(source.Repository, "bundles", "agent.yaml"), []byte("uncommitted invalid YAML"), 0600); err != nil {
				t.Fatal(err)
			}
			// Even a newer HEAD cannot move a pinned recipe.
			fixtureGit(t, source.Repository, "commit", "--quiet", "-am", "Changed bundle")
			got, network, err := ReadGitBundle(context.Background(), source, SHA256(data), true)
			if err != nil || network || !bytes.Equal(got, data) {
				t.Fatalf("network=%v err=%v", network, err)
			}
			lock := filepath.Join(t.TempDir(), "extensions.lock.yaml")
			if _, err = Install(lock, got, SHA256(data), false); err != nil {
				t.Fatal(err)
			}
			if err = os.RemoveAll(source.Repository); err != nil {
				t.Fatal(err)
			}
			registry, err := Load(lock)
			if err != nil {
				t.Fatal(err)
			}
			template, err := registry.Get("example-agent--evaluation-pass")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = registry.Render(template.ID, extensionTestValues(template)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGitBundleRejectsUnsafeInputsAndOfflineNetwork(t *testing.T) {
	source, data := gitBundleFixture(t, "sha1")
	cases := map[string]GitBundleSource{}
	for name, repository := range map[string]string{
		"HTTP": "http://example.invalid/repo.git", "SSH": "git@example.invalid:repo.git", "helper": "ext::sh -c bad", "userinfo": "https://secret@example.invalid/repo.git", "query": "https://example.invalid/repo.git?token=secret", "empty query": "https://example.invalid/repo.git?", "fragment": "https://example.invalid/repo.git#main", "offline HTTPS": "https://example.invalid/repo.git", "missing repository": "",
	} {
		s := source
		s.Repository = repository
		cases[name] = s
	}
	for _, commit := range []string{"main", "v1.0.0", source.Commit[:12], strings.ToUpper(source.Commit), strings.Repeat("f", 40), source.Commit + "^{commit}"} {
		s := source
		s.Commit = commit
		cases["commit "+commit] = s
	}
	for _, path := range []string{"", ".", "../agent.yaml", "/agent.yaml", "bundles/../agent.yaml", "bundles\\agent.yaml", ":(glob)**", "bundles/agent.yaml\n", "bundles", "absent.yaml"} {
		s := source
		s.Path = path
		cases["path "+path] = s
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := ReadGitBundle(context.Background(), s, SHA256(data), true); err == nil {
				t.Fatal("unsafe source accepted")
			} else if strings.Contains(err.Error(), "secret") {
				t.Fatal("credential leaked", err)
			}
		})
	}
	if _, _, err := ReadGitBundle(context.Background(), source, strings.Repeat("0", 64), true); err == nil {
		t.Fatal("wrong checksum accepted")
	}
	if _, _, err := ReadGitBundle(context.Background(), source, "invalid", true); err == nil {
		t.Fatal("invalid checksum accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := ReadGitBundle(ctx, source, SHA256(data), true); err == nil {
		t.Fatal("cancelled operation succeeded")
	}
}

func TestGitBundleRejectsSymlinksSubmodulesTagsAndLargeBlobs(t *testing.T) {
	source, data := gitBundleFixture(t, "sha1")
	if err := os.Symlink("bundles/agent.yaml", filepath.Join(source.Repository, "linked.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("bundles", filepath.Join(source.Repository, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source.Repository, "large.yaml"), bytes.Repeat([]byte("x"), MaxBundleBytes+1), 0600); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, source.Repository, "add", "linked.yaml", "linked-dir", "large.yaml")
	fixtureGit(t, source.Repository, "update-index", "--add", "--cacheinfo", "160000,"+source.Commit+",nested-repo")
	fixtureGit(t, source.Repository, "commit", "--quiet", "-m", "Unsafe entries")
	source.Commit = fixtureGit(t, source.Repository, "rev-parse", "HEAD")
	for _, path := range []string{"linked.yaml", "linked-dir/agent.yaml", "nested-repo", "nested-repo/bundles/agent.yaml", "large.yaml"} {
		t.Run(path, func(t *testing.T) {
			s := source
			s.Path = path
			if _, _, err := ReadGitBundle(context.Background(), s, SHA256(data), true); err == nil {
				t.Fatal("unsafe tree accepted")
			}
		})
	}
	fixtureGit(t, source.Repository, "tag", "-a", "v1", "-m", "Annotated tag")
	source.Commit = fixtureGit(t, source.Repository, "rev-parse", "v1")
	if _, _, err := ReadGitBundle(context.Background(), source, SHA256(data), true); err == nil {
		t.Fatal("tag object accepted as commit")
	}
}

func TestGitBundleIgnoresInheritedGitConfiguration(t *testing.T) {
	source, data := gitBundleFixture(t, "sha1")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "protocol.file.allow")
	t.Setenv("GIT_CONFIG_VALUE_0", "never")
	t.Setenv("GIT_DIR", "/nonexistent-git-dir")
	t.Setenv("GIT_OBJECT_DIRECTORY", "/nonexistent-objects")
	t.Setenv("GIT_CONFIG_SYSTEM", "/nonexistent-system-config")
	got, _, err := ReadGitBundle(context.Background(), source, SHA256(data), true)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("inherited Git configuration affected installation", err)
	}
}

func TestGitOutputBoundedForIOCopy(t *testing.T) {
	output := &gitOutput{limit: 3}
	if _, err := bytes.NewBufferString("oversized").WriteTo(output); err == nil || output.buffer.Len() > 3 {
		t.Fatal("Git output limit bypassed")
	}
}
