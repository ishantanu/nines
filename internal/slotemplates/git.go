package slotemplates

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

var gitCommitPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// GitBundleSource identifies committed YAML, never a branch, checkout or script.
// It is an installation receipt, not a runtime catalog dependency.
type GitBundleSource struct {
	Repository string `json:"repository" yaml:"repository"`
	Commit     string `json:"commit" yaml:"commit"`
	Path       string `json:"path" yaml:"path"`
}

func (source GitBundleSource) location(offline bool) (string, bool, error) {
	if !gitCommitPattern.MatchString(source.Commit) || !fs.ValidPath(source.Path) || source.Path == "." || len(source.Path) > 1024 || strings.ContainsAny(source.Path, "\\:\x00\r\n\t") {
		return "", false, model.Invalid("provide a full lowercase Git commit ID and a repository-relative regular bundle path")
	}
	repository := source.Repository
	if repository == "" || strings.ContainsAny(repository, "\x00\r\n") {
		return "", false, model.Invalid("provide a local repository path or public HTTPS Git URL")
	}
	if strings.HasPrefix(repository, "https://") {
		u, err := url.Parse(repository)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
			return "", false, model.Invalid("Git HTTPS URLs must not contain credentials, queries or fragments")
		}
		if offline {
			return "", false, model.Invalid("--offline prohibits remote Git fetches; use a local repository")
		}
		return repository, true, nil
	}
	// Colon excludes scp syntax, remote helpers, URL schemes and Windows drive paths
	// on Unix. Absolute Windows paths are accepted on Windows via filepath.IsAbs.
	if strings.Contains(repository, ":") && !filepath.IsAbs(repository) {
		return "", false, model.Invalid("only local repositories and public HTTPS Git URLs are supported")
	}
	absolute, err := filepath.Abs(repository)
	if err != nil {
		return "", false, model.Invalid("invalid local Git repository path")
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.IsDir() {
		return "", false, model.Invalid("local Git repository must be an existing directory")
	}
	return absolute, false, nil
}

// ReadGitBundle fetches one pinned commit into a disposable bare repository and
// reads a regular blob without checkout, filters, submodules or repository code.
// Neither the user's Git configuration nor Git credentials are inherited.
func ReadGitBundle(ctx context.Context, source GitBundleSource, expected string, offline bool) ([]byte, bool, error) {
	location, network, err := source.location(offline)
	if err != nil {
		return nil, false, err
	}
	if !digestPattern.MatchString(expected) {
		return nil, false, model.Invalid("provide an exact artifact SHA-256 before fetching Git")
	}
	executable, err := exec.LookPath("git")
	if err != nil {
		return nil, false, model.Invalid("Git installation requires git on PATH; local bundle rendering does not")
	}
	directory, err := os.MkdirTemp("", "nines-extension-git-")
	if err != nil {
		return nil, false, fmt.Errorf("cannot create temporary Git directory")
	}
	defer func() { _ = os.RemoveAll(directory) }()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	protocol := "file"
	if network {
		protocol = "https"
	}
	run := func(limit int, args ...string) ([]byte, error) {
		// No shell and no user-controlled executable/options. A private environment
		// also excludes GIT_CONFIG_COUNT, insteadOf rewrites, alternate object stores,
		// credential helpers, proxies and external diff/filter configuration.
		flags := []string{"--no-pager", "--literal-pathspecs", "-c", "protocol.allow=never", "-c", "protocol.file.allow=always", "-c", "protocol.https.allow=always", "-c", "http.followRedirects=false", "-c", "http.sslVerify=true", "-c", "credential.helper=", "-c", "core.hooksPath=" + directory, "-c", "maintenance.auto=false", "-c", "gc.auto=0"}
		command := exec.CommandContext(ctx, executable, append(flags, args...)...)
		command.Dir = directory
		command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + directory, "XDG_CONFIG_HOME=" + directory, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "GIT_NO_REPLACE_OBJECTS=1", "GIT_NO_LAZY_FETCH=1", "GIT_ALLOW_PROTOCOL=" + protocol, "GIT_LFS_SKIP_SMUDGE=1", "LC_ALL=C"}
		command.WaitDelay = time.Second
		output := &gitOutput{limit: limit}
		command.Stdout = output
		command.Stderr = io.Discard
		if err := command.Run(); err != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("extension Git operation cancelled or timed out")
			}
			return nil, model.Invalid("extension Git operation failed; verify repository access, commit and bundle path")
		}
		return output.Bytes(), nil
	}
	objectFormat := "sha1"
	if len(source.Commit) == 64 {
		objectFormat = "sha256"
	}
	if _, err = run(1024, "init", "--bare", "--quiet", "--template=", "--object-format="+objectFormat, "."); err != nil {
		return nil, network, err
	}
	if _, err = run(1024, "fetch", "--quiet", "--depth=1", "--no-tags", "--no-recurse-submodules", "--", location, source.Commit); err != nil {
		return nil, network, err
	}
	resolved, err := run(256, "rev-parse", "--verify", "FETCH_HEAD^{commit}")
	if err != nil || strings.TrimSpace(string(resolved)) != source.Commit {
		return nil, network, model.Invalid("Git source must identify the exact fetched commit, not a tag or ref")
	}
	entry, err := run(2048, "ls-tree", "-z", source.Commit, "--", source.Path)
	if err != nil {
		return nil, network, err
	}
	fields, path, ok := strings.Cut(string(entry), "\t")
	parts := strings.Fields(fields)
	if !ok || path != source.Path+"\x00" || len(parts) != 3 || parts[1] != "blob" || (parts[0] != "100644" && parts[0] != "100755") || !gitCommitPattern.MatchString(parts[2]) {
		return nil, network, model.Invalid("Git bundle path must identify a regular committed file, not a symlink or submodule")
	}
	sizeOutput, err := run(128, "cat-file", "-s", parts[2])
	if err != nil {
		return nil, network, err
	}
	size, err := strconv.ParseInt(strings.TrimSpace(string(sizeOutput)), 10, 64)
	if err != nil || size < 1 || size > MaxBundleBytes {
		return nil, network, model.Invalid("Git bundle must contain between 1 byte and 1 MiB")
	}
	data, err := run(MaxBundleBytes, "cat-file", "blob", parts[2])
	if err != nil {
		return nil, network, err
	}
	if int64(len(data)) != size || SHA256(data) != expected {
		return nil, network, model.Invalid("Git bundle checksum mismatch")
	}
	return data, network, nil
}

type gitOutput struct {
	buffer bytes.Buffer
	limit  int
}

func (output *gitOutput) Write(data []byte) (int, error) {
	if len(data) > output.limit-output.buffer.Len() {
		return 0, fmt.Errorf("git output exceeds limit")
	}
	return output.buffer.Write(data)
}

func (output *gitOutput) Bytes() []byte { return output.buffer.Bytes() }
