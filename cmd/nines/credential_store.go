package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/zalando/go-keyring"
)

type credentialBackend interface {
	Get(string, string) (string, error)
	Set(string, string, string) error
	Delete(string, string) error
}
type systemKeyring struct{}

func (systemKeyring) Get(service, account string) (string, error) {
	return keyring.Get(service, account)
}
func (systemKeyring) Set(service, account, value string) error {
	return keyring.Set(service, account, value)
}
func (systemKeyring) Delete(service, account string) error { return keyring.Delete(service, account) }

var osCredentials credentialBackend = systemKeyring{}

const credentialService = "nines.cli.v1"

func credentialAccount(path string, c credential) string {
	b, _ := json.Marshal([]string{path, c.Server, c.Tenant, c.Identity, c.KeychainID})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func storedToken(path string, c credential, agent bool) (string, error) {
	if c.Storage == "" || c.Storage == "file" {
		if len(c.Token) > 4096 || strings.ContainsAny(c.Token, "\r\n") {
			return "", usage("invalid saved CLI credential")
		}
		return c.Token, nil
	}
	if c.Storage != "keychain" || c.Identity == "" || c.KeychainID == "" || c.Token != "" {
		return "", usage("invalid credential-store metadata")
	}
	if agent {
		return "", &cliError{"unauthenticated", "Agent mode never unlocks an OS credential store; supply NINES_API_KEY", 3}
	}
	raw, e := osCredentials.Get(credentialService, credentialAccount(path, c))
	if e != nil {
		return "", &cliError{"unavailable", "OS credential store unavailable; unlock it manually or use NINES_API_KEY", 6}
	}
	var secret credential
	if json.Unmarshal([]byte(raw), &secret) != nil || secret.Server != c.Server || secret.Tenant != c.Tenant || secret.Identity != c.Identity || secret.KeychainID != c.KeychainID || !secret.Expires.Equal(c.Expires) || secret.Token == "" || len(secret.Token) > 4096 || strings.ContainsAny(secret.Token, "\r\n") {
		return "", usage("OS credential does not match the selected context")
	}
	return secret.Token, nil
}
func secureCredential(path string, c credential, storage string) (credential, error) {
	if storage == "file" {
		c.Storage = "file"
		return c, nil
	}
	if storage != "keychain" {
		return c, usage("credential-store must be file or keychain")
	}
	if c.Identity == "" {
		return c, usage("server did not supply a CLI identity; use file storage or an API key")
	}
	id, e := nonce()
	if e != nil {
		return c, e
	}
	c.Storage = "keychain"
	c.KeychainID = id
	data, e := json.Marshal(c)
	if e != nil {
		return c, e
	}
	if e = osCredentials.Set(credentialService, credentialAccount(path, c), string(data)); e != nil {
		return c, &cliError{"unavailable", "OS credential store unavailable; no file fallback was used. Retry with --credential-store file or NINES_API_KEY", 6}
	}
	c.Token = ""
	return c, nil
}
func deleteStoredCredential(path string, c credential, agent bool) error {
	if c.Storage != "keychain" {
		return nil
	}
	if agent {
		return &cliError{"unauthenticated", "Agent mode cannot modify an OS credential store; run logout interactively", 3}
	}
	err := osCredentials.Delete(credentialService, credentialAccount(path, c))
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	if err != nil {
		return &cliError{"unavailable", "Unable to remove OS credential; retry logout after unlocking the credential store", 6}
	}
	return nil
}
func credentialSnapshot(path, name string) (string, error) {
	cfg, e := configLoad(path)
	if e != nil {
		return "", e
	}
	credentials, e := credentialsLoad(path)
	if e != nil {
		return "", e
	}
	entry, exists := cfg.Contexts[name]
	raw, _ := json.Marshal(struct {
		Context    savedContext
		Exists     bool
		Credential credential
	}{entry, exists, credentials[name]})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
