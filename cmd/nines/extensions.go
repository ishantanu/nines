package main

import (
	"io"

	"github.com/spf13/cobra"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/slotemplates"
)

func readTemplateValues(file string) (v1.TemplateValues, error) {
	var values v1.TemplateValues
	if file == "" {
		return values, nil
	}
	f, err := openInput(file)
	if err != nil {
		return values, err
	}
	defer func() { _ = f.Close() }()
	body, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil {
		return values, err
	}
	if len(body) > 64<<10 {
		return values, usage("template values exceed 64 KiB")
	}
	return slotemplates.ParseValues(body)
}
func extensionsCommand(o *options) *cobra.Command {
	group := &cobra.Command{Use: "extensions", Short: "Validate, test and install declarative SLO extensions"}
	for _, action := range []string{"validate", "test"} {
		var file string
		cmd := &cobra.Command{Use: action, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			if file == "" {
				return usage("provide --file bundle.yaml")
			}
			data, err := slotemplates.ReadBundleFile(file)
			if err != nil {
				return err
			}
			b, err := slotemplates.ParseBundle(data)
			if err != nil {
				return err
			}
			return render(cmd.OutOrStdout(), map[string]any{"apiVersion": v1.APIVersion, "kind": "SLOExtensionValidation", "name": b.Metadata.Name, "version": b.Metadata.Version, "sha256": slotemplates.SHA256(data), "engine": b.Spec.Engine, "recipes": len(b.Spec.Recipes), "testsPassed": len(b.Spec.Tests), "status": "passed", "networkUsed": false}, o.format("json"), "extension")
		}}
		cmd.Flags().StringVarP(&file, "file", "f", "", "Declarative extension bundle YAML")
		group.AddCommand(mark(cmd, "offline", "read", "Validate schema and run deterministic extension conformance cases"))
	}
	var lock string
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		registry, err := slotemplates.Load(lock)
		if err != nil {
			return err
		}
		catalog, err := registry.List("")
		if err != nil {
			return err
		}
		return render(cmd.OutOrStdout(), map[string]any{"apiVersion": v1.APIVersion, "kind": "SLOExtensionInventory", "extensions": catalog.Extensions, "catalogDigest": catalog.Digest}, o.format("json"), "extensions")
	}}
	list.Flags().StringVar(&lock, "lock", "", "Pinned extension lockfile")
	group.AddCommand(mark(list, "offline", "read", "List installed, validated extension versions and checksums"))
	var file, endpoint, expected, destination string
	var gitSource slotemplates.GitBundleSource
	var replace bool
	install := &cobra.Command{Use: "install", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		sources := 0
		for _, value := range []string{file, endpoint, gitSource.Repository} {
			if value != "" {
				sources++
			}
		}
		if sources != 1 || destination == "" || expected == "" {
			return usage("provide exactly one --file, --url or --git, plus --sha256 and --lock")
		}
		if gitSource.Repository == "" && (gitSource.Commit != "" || gitSource.Path != "") {
			return usage("--commit and --git-path require --git")
		}
		if endpoint != "" && o.Offline {
			return usage("--offline prohibits extension downloads; use --file")
		}
		var data []byte
		var err error
		network := endpoint != ""
		if gitSource.Repository != "" {
			data, network, err = slotemplates.ReadGitBundle(cmd.Context(), gitSource, expected, o.Offline)
		} else if endpoint != "" {
			data, err = slotemplates.FetchBundle(cmd.Context(), endpoint, expected)
		} else {
			data, err = slotemplates.ReadBundleFile(file)
		}
		if err != nil {
			return err
		}
		info, err := slotemplates.Install(destination, data, expected, replace)
		if err != nil {
			return err
		}
		receipt := map[string]any{"apiVersion": v1.APIVersion, "kind": "SLOExtensionInstallation", "extension": info, "lock": destination, "networkUsed": network, "status": "installed"}
		if gitSource.Repository != "" {
			receipt["git"] = gitSource
		}
		return render(cmd.OutOrStdout(), receipt, o.format("json"), "extension")
	}}
	install.Flags().StringVarP(&file, "file", "f", "", "Local regular bundle file")
	install.Flags().StringVar(&endpoint, "url", "", "Explicit HTTPS bundle artifact URL; never accessed during rendering")
	install.Flags().StringVar(&gitSource.Repository, "git", "", "Local Git repository or public HTTPS Git URL")
	install.Flags().StringVar(&gitSource.Commit, "commit", "", "Full lowercase Git commit ID; required with --git")
	install.Flags().StringVar(&gitSource.Path, "git-path", "", "Repository-relative bundle YAML path; required with --git")
	install.Flags().StringVar(&expected, "sha256", "", "Expected exact lowercase artifact SHA-256, obtained independently")
	install.Flags().StringVar(&destination, "lock", "", "Local lockfile to create/update; its parent must exist")
	install.Flags().BoolVar(&replace, "replace", false, "Explicitly replace an extension with a different version")
	mark(install, "offline", "local-write", "Install a checksum-pinned bundle; --url or remote --git explicitly reads the network")
	install.Annotations["file-alternatives"] = "url,git"
	install.MarkFlagsOneRequired("file", "url", "git")
	install.MarkFlagsRequiredTogether("git", "commit", "git-path")
	install.MarkFlagsMutuallyExclusive("file", "url", "git")
	group.AddCommand(install)
	return group
}
