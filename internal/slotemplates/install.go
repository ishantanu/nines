package slotemplates

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func readBounded(reader io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(reader, MaxBundleBytes+1))
	if err != nil || len(b) > MaxBundleBytes {
		return nil, model.Invalid("extension read failed or exceeded 1 MiB")
	}
	return b, nil
}

// ReadBundleFile refuses symlinks and non-regular input files.
func ReadBundleFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, model.Invalid("extension input must be a regular file, not a symlink")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, model.Invalid("extension input changed while opening")
	}
	return readBounded(f)
}
func parseLock(data []byte) (v1.SLOExtensionLock, error) {
	var lock v1.SLOExtensionLock
	if err := strictDocument(data, &lock); err != nil {
		return lock, err
	}
	if lock.APIVersion != v1.APIVersion || lock.Kind != "SLOExtensionLock" || len(lock.Extensions) > 32 {
		return lock, model.Invalid("invalid extension lock version, kind or size")
	}
	seen := map[string]bool{}
	for _, entry := range lock.Extensions {
		if !extensionName.MatchString(entry.Name) || seen[entry.Name] || !validExtensionVersion(entry.Version) || !digestPattern.MatchString(entry.SHA256) || !fs.ValidPath(entry.Path) || strings.Contains(entry.Path, "\\") {
			return lock, model.Invalid("invalid or duplicate extension lock entry")
		}
		seen[entry.Name] = true
	}
	return lock, nil
}
func rootRead(root *os.Root, path string) ([]byte, error) {
	// Check every component, then use Go's root-scoped open to prevent path escape
	// even if an ancestor is concurrently replaced with a symlink.
	pieces := strings.Split(path, "/")
	prefix := ""
	for index, piece := range pieces {
		if prefix != "" {
			prefix += "/"
		}
		prefix += piece
		info, err := root.Lstat(prefix)
		if err != nil {
			return nil, err
		}
		if index == len(pieces)-1 && !info.Mode().IsRegular() || index < len(pieces)-1 && !info.IsDir() {
			return nil, model.Invalid("extension lock paths require regular files and directory ancestors")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, model.Invalid("extension lock paths must not contain symlinks")
		}
	}
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, model.Invalid("extension lock path must identify a regular file")
	}
	return readBounded(f)
}

// Load reads only administrator-installed, checksum-pinned files. No registry
// request is made during startup, rendering or operator reconciliation.
func Load(lockPath string) (*Registry, error) {
	registry := Default()
	if lockPath == "" {
		return registry, nil
	}
	root, err := os.OpenRoot(filepath.Dir(lockPath))
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	data, err := rootRead(root, filepath.Base(lockPath))
	if err != nil {
		return nil, err
	}
	lock, err := parseLock(data)
	if err != nil {
		return nil, err
	}
	sort.Slice(lock.Extensions, func(i, j int) bool { return lock.Extensions[i].Name < lock.Extensions[j].Name })
	for _, entry := range lock.Extensions {
		data, err := rootRead(root, entry.Path)
		if err != nil {
			return nil, fmt.Errorf("extension %s cannot be read: %w", entry.Name, err)
		}
		if SHA256(data) != entry.SHA256 {
			return nil, model.Invalid("extension %s checksum mismatch", entry.Name)
		}
		b, err := ParseBundle(data)
		if err != nil {
			return nil, err
		}
		if b.Metadata.Name != entry.Name || b.Metadata.Version != entry.Version {
			return nil, model.Invalid("extension lock identity does not match artifact")
		}
		if err = registry.add(b, entry.SHA256); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// FetchBundle is an explicit CLI installation step. HTTPS only, no redirects,
// inherited credentials, automatic discovery or requests from the rendering path.
func FetchBundle(ctx context.Context, endpoint, expected string) ([]byte, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	return fetchBundle(ctx, endpoint, expected, transport)
}
func fetchBundle(ctx context.Context, endpoint, expected string, transport *http.Transport) ([]byte, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || !digestPattern.MatchString(expected) {
		return nil, model.Invalid("provide an HTTPS artifact URL without credentials/query and an exact SHA-256")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, model.Invalid("invalid extension download request")
	}
	client := http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("extension artifact request failed")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("extension artifact request returned HTTP %d", response.StatusCode)
	}
	data, err := readBounded(response.Body)
	if err != nil {
		return nil, err
	}
	if SHA256(data) != expected {
		return nil, model.Invalid("downloaded extension checksum mismatch")
	}
	return data, nil
}

// Install validates before mutation, stores immutable content-addressed YAML and
// atomically replaces the lockfile. The exclusive guard serializes CLI installers.
func Install(lockPath string, data []byte, expected string, replace bool) (v1.SLOExtensionInfo, error) {
	var empty v1.SLOExtensionInfo
	if !digestPattern.MatchString(expected) || SHA256(data) != expected {
		return empty, model.Invalid("provide the exact expected artifact SHA-256")
	}
	bundle, err := ParseBundle(data)
	if err != nil {
		return empty, err
	}
	root, err := os.OpenRoot(filepath.Dir(lockPath))
	if err != nil {
		return empty, err
	}
	defer func() { _ = root.Close() }()
	name := filepath.Base(lockPath)
	if name == "." || name == "/" || strings.ContainsAny(name, "\\") {
		return empty, model.Invalid("invalid extension lock path")
	}
	guard, err := root.OpenFile(name+".installing", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return empty, fmt.Errorf("extension installation already active or lock directory unwritable")
	}
	defer func() { _ = guard.Close(); _ = root.Remove(name + ".installing") }()
	before, err := rootRead(root, name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return empty, err
	}
	candidate := Default()
	lock := v1.SLOExtensionLock{APIVersion: v1.APIVersion, Kind: "SLOExtensionLock", Extensions: []v1.ExtensionLockEntry{}}
	if err == nil {
		lock, err = parseLock(before)
		if err != nil {
			return empty, err
		}
		if candidate, err = Load(lockPath); err != nil {
			return empty, err
		}
	}
	found := -1
	for index, entry := range lock.Extensions {
		if entry.Name == bundle.Metadata.Name {
			if entry.Version == bundle.Metadata.Version && entry.SHA256 != expected {
				return empty, model.Invalid("an extension version cannot change content; publish a new version")
			}
			if entry.SHA256 != expected && !replace {
				return empty, model.Invalid("extension upgrade requires --replace and a new version")
			}
			found = index
		}
	}
	if found < 0 && len(lock.Extensions) >= 32 {
		return empty, model.Invalid("at most 32 installed extensions are supported")
	}
	keptTemplates := candidate.templates[:0]
	for _, t := range candidate.templates {
		if t.Extension == nil || t.Extension.Name != bundle.Metadata.Name {
			keptTemplates = append(keptTemplates, t)
		}
	}
	candidate.templates = keptTemplates
	keptCategories := candidate.categories[:0]
	for _, c := range candidate.categories {
		if !strings.HasPrefix(c.ID, bundle.Metadata.Name+"--") {
			keptCategories = append(keptCategories, c)
		}
	}
	candidate.categories = keptCategories
	keptExtensions := candidate.extensions[:0]
	for _, p := range candidate.extensions {
		if p.Name != bundle.Metadata.Name {
			keptExtensions = append(keptExtensions, p)
		}
	}
	candidate.extensions = keptExtensions
	if err = candidate.add(bundle, expected); err != nil {
		return empty, err
	}
	if err = root.Mkdir("nines-extensions", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return empty, err
	}
	info, err := root.Lstat("nines-extensions")
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return empty, model.Invalid("extension storage must be a directory, not a symlink")
	}
	path := "nines-extensions/" + expected + ".yaml"
	f, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		stored, e := rootRead(root, path)
		if e != nil || !bytes.Equal(stored, data) {
			return empty, model.Invalid("existing extension artifact is not the pinned content")
		}
	} else if err != nil {
		return empty, err
	} else {
		_, writeErr := f.Write(data)
		syncErr := f.Sync()
		closeErr := f.Close()
		if writeErr != nil || syncErr != nil || closeErr != nil {
			_ = root.Remove(path)
			return empty, fmt.Errorf("could not persist extension artifact")
		}
	}
	entry := v1.ExtensionLockEntry{Name: bundle.Metadata.Name, Version: bundle.Metadata.Version, SHA256: expected, Path: path}
	if found < 0 {
		lock.Extensions = append(lock.Extensions, entry)
	} else {
		lock.Extensions[found] = entry
	}
	sort.Slice(lock.Extensions, func(i, j int) bool { return lock.Extensions[i].Name < lock.Extensions[j].Name })
	body, err := yaml.Marshal(lock)
	if err != nil {
		return empty, err
	}
	// The guard excludes cooperating installers. Refuse external manual edits too.
	current, readErr := rootRead(root, name)
	if !bytes.Equal(current, before) || readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return empty, model.ErrRevisionConflict
	}
	temporary := name + ".pending"
	output, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return empty, err
	}
	defer func() { _ = root.Remove(temporary) }()
	_, writeErr := output.Write(body)
	syncErr := output.Sync()
	closeErr := output.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return empty, fmt.Errorf("could not persist extension lock")
	}
	if err = root.Rename(temporary, name); err != nil {
		return empty, err
	}
	return extensionInfo(bundle, expected), nil
}
