package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/tenancy"
)

type savedContext struct {
	Server string `json:"server" yaml:"server"`
	Tenant string `json:"tenant" yaml:"tenant"`
}
type savedConfig struct {
	APIVersion string                  `json:"apiVersion" yaml:"apiVersion"`
	Kind       string                  `json:"kind" yaml:"kind"`
	Current    string                  `json:"currentContext,omitempty" yaml:"currentContext,omitempty"`
	Contexts   map[string]savedContext `json:"contexts" yaml:"contexts"`
}
type credential struct {
	Server  string    `json:"server"`
	Tenant  string    `json:"tenant"`
	Token   string    `json:"token"`
	Expires time.Time `json:"expiresAt"`
}

func (o *options) configPath() (string, error) {
	p := o.Config
	if p == "" {
		p = os.Getenv("NINES_CONFIG")
	}
	if p != "" {
		return filepath.Abs(p)
	}
	d, e := os.UserConfigDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(d, "nines", "config.yaml"), nil
}
func privateRead(path string, limit int64) ([]byte, error) {
	st, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
		return nil, usage("CLI configuration and credentials must be regular files with mode 0600")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer func() {
		_ = f.Close()
	}()
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) > limit {
		return nil, usage("CLI configuration exceeds size limit")
	}
	return b, nil
}
func configLoad(path string) (savedConfig, error) {
	c := savedConfig{APIVersion: "nines.dev/v1alpha1", Kind: "CLIConfig", Contexts: map[string]savedContext{}}
	b, e := privateRead(path, 64<<10)
	if errors.Is(e, os.ErrNotExist) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if e = d.Decode(&c); e != nil {
		return c, usage("invalid CLI configuration")
	}
	if d.Decode(new(any)) != io.EOF || c.APIVersion != "nines.dev/v1alpha1" || c.Kind != "CLIConfig" || len(c.Contexts) > 100 {
		return c, usage("invalid CLI configuration envelope")
	}
	if c.Contexts == nil {
		c.Contexts = map[string]savedContext{}
	}
	for name, v := range c.Contexts {
		if !tenancy.ValidID(name) {
			return c, usage("invalid context name")
		}
		if _, e = client.New(client.Config{URL: v.Server, Tenant: v.Tenant}); e != nil {
			return c, usage("invalid context server or tenant")
		}
	}
	return c, nil
}
func withConfigLock(path string, fn func() error) (resultErr error) {
	d := filepath.Dir(path)
	if e := os.MkdirAll(d, 0700); e != nil {
		return e
	}
	st, e := os.Lstat(d)
	if e != nil {
		return e
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return usage("CLI configuration directory must be private with mode 0700")
	}
	f, e := os.OpenFile(path+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return &cliError{"conflict", "CLI configuration is being updated; retry after the other command completes", 5}
	}
	defer func() {
		resultErr = errors.Join(resultErr, os.Remove(path+".lock"))
	}()
	if err := f.Close(); err != nil {
		return err
	}
	return fn()
}
func privateWrite(path string, b []byte) error {
	if st, e := os.Lstat(path); e == nil && (!st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0) {
		return usage("refusing unsafe CLI configuration path")
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".nines-*")
	if e != nil {
		return e
	}
	defer func() {
		_ = os.Remove(f.Name())
	}()
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(f.Name(), path)
}
func configSave(path string, c savedConfig) error {
	b, e := yaml.Marshal(c)
	if e != nil {
		return e
	}
	return privateWrite(path, b)
}
func credentialsLoad(path string) (map[string]credential, error) {
	m := map[string]credential{}
	b, e := privateRead(path+".credentials.json", 128<<10)
	if errors.Is(e, os.ErrNotExist) {
		return m, nil
	}
	if e != nil {
		return nil, e
	}
	if e = json.Unmarshal(b, &m); e != nil || len(m) > 100 {
		return nil, usage("invalid CLI credential store")
	}
	return m, nil
}
func credentialsSave(path string, m map[string]credential) error {
	b, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return e
	}
	return privateWrite(path+".credentials.json", append(b, '\n'))
}
func environmentToken() (string, string) {
	if v := os.Getenv("NINES_API_KEY"); v != "" {
		return v, "NINES_API_KEY"
	}
	if v := os.Getenv("API_TOKEN"); v != "" {
		return v, "API_TOKEN"
	}
	return "", "none"
}
func (o *options) resolve(cmd *cobra.Command) error {
	p, e := o.configPath()
	if e != nil {
		return e
	}
	c, e := configLoad(p)
	if e != nil {
		return e
	}
	if o.Context == "" {
		o.Context = os.Getenv("NINES_CONTEXT")
	}
	if o.Context == "" {
		o.Context = c.Current
	}
	v, exists := c.Contexts[o.Context]
	if o.Context != "" && !exists && cmd.Name() != "login" {
		return usage("context does not exist; use config set-context")
	}
	if o.Server == "" {
		o.Server = os.Getenv("NINES_URL")
	}
	if o.Server == "" {
		o.Server = v.Server
	}
	if o.Server == "" {
		o.Server = "http://127.0.0.1:8080"
	}
	o.Server = strings.TrimRight(o.Server, "/")
	if o.Tenant == "" {
		o.Tenant = os.Getenv("NINES_TENANT")
	}
	if o.Tenant == "" {
		o.Tenant = v.Tenant
	}
	if o.Tenant == "" {
		o.Tenant = "default"
	}
	o.Token, o.credentialSource = environmentToken()
	if _, e = client.New(client.Config{URL: o.Server, Tenant: o.Tenant, Token: o.Token}); e != nil {
		return usage(e.Error())
	}
	if o.Token == "" && o.Context != "" {
		m, e := credentialsLoad(p)
		if e != nil {
			return e
		}
		v, ok := m[o.Context]
		if ok && v.Server == o.Server && v.Tenant == o.Tenant {
			if !v.Expires.After(time.Now()) && cmd.Name() != "login" && cmd.Name() != "logout" {
				return &cliError{"unauthenticated", "CLI session expired; run nines login or set NINES_API_KEY", 3}
			}
			o.Token = v.Token
			o.credentialSource = "browser-session"
		}
	}
	return nil
}
func configCommand(o *options) *cobra.Command {
	c := &cobra.Command{Use: "config", Short: "Manage named server and tenant contexts"}
	c.AddCommand(mark(&cobra.Command{Use: "set-context NAME", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !tenancy.ValidID(args[0]) || o.Server == "" || !cmd.Flags().Changed("tenant") && !cmd.InheritedFlags().Changed("tenant") {
			return usage("set-context requires NAME, --server and --tenant")
		}
		if _, e := client.New(client.Config{URL: o.Server, Tenant: o.Tenant}); e != nil {
			return usage(e.Error())
		}
		p, e := o.configPath()
		if e != nil {
			return e
		}
		e = withConfigLock(p, func() error {
			c, e := configLoad(p)
			if e != nil {
				return e
			}
			c.Contexts[args[0]] = savedContext{strings.TrimRight(o.Server, "/"), o.Tenant}
			if c.Current == "" {
				c.Current = args[0]
			}
			return configSave(p, c)
		})
		if e != nil {
			return e
		}
		return render(cmd.OutOrStdout(), map[string]string{"context": args[0], "server": o.Server, "tenant": o.Tenant}, o.format("json"), "config")
	}}, "offline", "local-write", "Save a server/tenant context"), mark(&cobra.Command{Use: "use-context NAME", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		p, e := o.configPath()
		if e != nil {
			return e
		}
		e = withConfigLock(p, func() error {
			c, e := configLoad(p)
			if e != nil {
				return e
			}
			if _, ok := c.Contexts[args[0]]; !ok {
				return usage("context does not exist")
			}
			c.Current = args[0]
			return configSave(p, c)
		})
		if e != nil {
			return e
		}
		return render(cmd.OutOrStdout(), map[string]string{"currentContext": args[0]}, o.format("json"), "config")
	}}, "offline", "local-write", "Select the default context"), mark(&cobra.Command{Use: "get-contexts", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		p, e := o.configPath()
		if e != nil {
			return e
		}
		c, e := configLoad(p)
		if e != nil {
			return e
		}
		return render(cmd.OutOrStdout(), c, o.format("yaml"), "config")
	}}, "offline", "read", "List contexts without reading credentials"))
	return c
}
