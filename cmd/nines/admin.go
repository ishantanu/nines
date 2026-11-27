package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ishantanu/nines/client"
)

// adminCommand is platform administration: tenants across the server. It
// needs a platform administrator signed in with `nines auth login`.
func adminCommand(o *options) *cobra.Command {
	admin := &cobra.Command{Use: "admin", Short: "Platform administration (platform administrators only)"}
	tenants := &cobra.Command{Use: "tenants", Short: "Create, rename, retire and delete tenants"}
	admin.AddCommand(tenants)
	api := func() (*client.Client, error) {
		return client.New(client.Config{URL: o.Server, Tenant: o.Tenant, Token: o.Token})
	}

	tenants.AddCommand(mark(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		a, err := api()
		if err != nil {
			return err
		}
		out, err := a.ListPlatformTenants(c.Context())
		if err != nil {
			return err
		}
		if format := o.format("table"); format != "table" {
			return render(c.OutOrStdout(), out, format, "tenants")
		}
		w := tabwriter.NewWriter(c.OutOrStdout(), 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "ID\tNAME\tMANAGED BY\tSTATE\tVERSION")
		for _, t := range out.Tenants {
			state, version := t.State, fmt.Sprint(t.Version)
			if t.ManagedBy == "configuration" {
				state, version = "active", "-"
			}
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.ID, t.Name, t.ManagedBy, state, version)
		}
		return w.Flush()
	}}, "server", "read", "List every tenant on the server"))

	var reason string
	reasonFlag := func(c *cobra.Command) {
		c.Flags().StringVar(&reason, "reason", "", "why, recorded in the platform audit log (required)")
		_ = c.MarkFlagRequired("reason")
	}
	var expected int64
	versionFlag := func(c *cobra.Command) {
		c.Flags().Int64Var(&expected, "expected-version", 0, "refuse if the tenant changed since this version (default: its current version)")
	}
	// version is the expected version: as given, or the tenant's current one.
	version := func(ctx context.Context, a *client.Client, id string) (int64, error) {
		if expected > 0 {
			return expected, nil
		}
		list, err := a.ListPlatformTenants(ctx)
		if err != nil {
			return 0, err
		}
		for _, t := range list.Tenants {
			if t.ID == id {
				if t.ManagedBy != "console" {
					return 0, usage("tenant " + id + " is managed by server configuration; change it there")
				}
				return t.Version, nil
			}
		}
		return 0, usage("no tenant " + id)
	}

	var in client.NewTenant
	var owner client.NewLocalOwner
	var passwordStdin bool
	create := &cobra.Command{Use: "create ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		in.ID = args[0]
		local := owner.Username != ""
		if local == (in.OwnerSubject != "") {
			return usage("choose the first owner: --owner-username for a new local account, or --owner-provider and --owner-subject for someone who can already sign in")
		}
		if local {
			if !passwordStdin {
				return usage("a new local owner needs --owner-password-stdin; the password is never taken as a flag")
			}
			password, err := readLine(passwordInput())
			if err != nil {
				return usage("read the owner's initial password from stdin")
			}
			owner.Password = password
			if owner.Provider == "" {
				owner.Provider = "local"
			}
			if owner.Name == "" {
				owner.Name = owner.Username
			}
			in.OwnerUser = &owner
		}
		a, err := api()
		if err != nil {
			return err
		}
		out, err := a.CreateTenant(c.Context(), in, reason)
		owner.Password = ""
		if err != nil {
			return err
		}
		return render(c.OutOrStdout(), out, o.format("json"), "tenant")
	}}
	f := create.Flags()
	f.StringVar(&in.Name, "name", "", "display name (required)")
	_ = create.MarkFlagRequired("name")
	f.StringVar(&owner.Username, "owner-username", "", "create the first owner as a new local account with this username")
	f.StringVar(&owner.Name, "owner-name", "", "the new local owner's display name (default: the username)")
	f.StringVar(&owner.Email, "owner-email", "", "the new local owner's email")
	f.BoolVar(&passwordStdin, "owner-password-stdin", false, "read the new local owner's initial password (12 to 72 characters) from stdin")
	f.StringVar(&in.OwnerProvider, "owner-provider", "", "sign-in provider of an existing person to make owner (with --owner-subject)")
	f.StringVar(&in.OwnerSubject, "owner-subject", "", "that person's user ID at the provider")
	reasonFlag(create)
	tenants.AddCommand(mark(create, "server", "write", "Create a tenant and its first owner"))

	var name string
	rename := &cobra.Command{Use: "rename ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		a, err := api()
		if err != nil {
			return err
		}
		v, err := version(c.Context(), a, args[0])
		if err != nil {
			return err
		}
		out, err := a.RenameTenant(c.Context(), args[0], name, v, reason)
		if err != nil {
			return err
		}
		return render(c.OutOrStdout(), out, o.format("json"), "tenant")
	}}
	rename.Flags().StringVar(&name, "name", "", "new display name (required); the ID does not change")
	_ = rename.MarkFlagRequired("name")
	reasonFlag(rename)
	versionFlag(rename)
	tenants.AddCommand(mark(rename, "server", "write", "Change a tenant's display name"))

	retire := &cobra.Command{Use: "retire ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		a, err := api()
		if err != nil {
			return err
		}
		v, err := version(c.Context(), a, args[0])
		if err != nil {
			return err
		}
		out, err := a.RetireTenant(c.Context(), args[0], v, reason)
		if err != nil {
			return err
		}
		return render(c.OutOrStdout(), out, o.format("json"), "tenant")
	}}
	reasonFlag(retire)
	versionFlag(retire)
	tenants.AddCommand(mark(retire, "server", "write", "Stop a tenant's collection and alerting and make it read-only"))

	var confirm string
	del := &cobra.Command{Use: "delete ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		if confirm != args[0] {
			return usage("deleting a tenant permanently deletes its data; repeat its ID with --confirm-id")
		}
		a, err := api()
		if err != nil {
			return err
		}
		v, err := version(c.Context(), a, args[0])
		if err != nil {
			return err
		}
		if err = a.DeleteTenant(c.Context(), args[0], confirm, v, reason); err != nil {
			return err
		}
		return render(c.OutOrStdout(), map[string]string{"tenant": args[0], "status": "deleted"}, o.format("json"), "tenant")
	}}
	del.Flags().StringVar(&confirm, "confirm-id", "", "the tenant ID again, to confirm permanent deletion (required)")
	reasonFlag(del)
	versionFlag(del)
	tenants.AddCommand(mark(del, "server", "write", "Permanently delete a retired tenant and its data"))
	return admin
}

// passwordInput is where a new local owner's password is read; tests replace it.
var passwordInput = func() io.Reader { return os.Stdin }

// readLine reads one line, such as a password piped on stdin.
func readLine(r io.Reader) (string, error) {
	line, err := bufio.NewReader(io.LimitReader(r, 4096)).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return "", fmt.Errorf("empty input")
	}
	return line, nil
}
