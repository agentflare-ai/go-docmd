package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	cobradoc "github.com/spf13/cobra/doc"
)

const rootLongDesc = `
go-docmd is a drop-in companion to go doc that renders Markdown instead of plaintext.
It mirrors the go doc argument patterns (pkg, pkg.Type, pkg.Type.Method), emits README-friendly
output for entire module trees, and now ships with a Cobra-powered CLI that includes:

  • Rich, structured help text and version info (` + "`go-docmd --help`" + `, ` + "`go-docmd --version`" + `)
  • Shell completion generation for bash, zsh, fish, and PowerShell
  • A gen-docs helper that can emit Markdown reference docs for the CLI itself

Use go run ./go-docmd just like go doc, or install the binary and enjoy autocompletion +
CLI docs generation in your release workflows.
`

func newRootCmd(stdout io.Writer) *cobra.Command {
	app := &cliApp{stdout: stdout}
	cmd := &cobra.Command{
		Use:           "go-docmd [flags] [package|[package.]symbol[.method]]",
		Short:         "Render Go documentation as Markdown",
		Long:          strings.TrimSpace(rootLongDesc),
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.DisableAutoGenTag = true
	cmd.Version = Version
	cmd.SetOut(stdout)
	cmd.SetErr(io.Discard)
	cmd.CompletionOptions.DisableDefaultCmd = true

	flags := cmd.Flags()
	flags.BoolVar(&app.opts.all, "all", false, "show all documentation for package")
	flags.BoolVarP(&app.opts.caseSensitive, "case-sensitive", "c", false, "symbol matching honors case (paths not affected)")
	flags.BoolVar(&app.opts.showCmd, "cmd", false, "show symbols for package main")
	flags.BoolVar(&app.opts.short, "short", false, "one-line representation for each symbol")
	flags.BoolVar(&app.opts.showSource, "src", false, "show source code for the matched declaration")
	flags.BoolVarP(&app.opts.unexported, "unexported", "u", false, "show unexported symbols as well as exported")
	flags.StringVarP(&app.opts.outputPath, "output", "o", "", "write output Markdown to file instead of stdout")
	flags.BoolVar(&app.opts.inplace, "inplace", false, "write README.md directly into package directories (overwrites existing files)")
	flags.BoolVar(&app.opts.includeMainVars, "mainvars", false, "include variable listings in package main output")
	flags.BoolVar(&app.opts.includeMainFuncs, "mainfuncs", false, "include function listings in package main output")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		return app.execute(ctx, args)
	}

	cmd.AddCommand(newCompletionCmd(cmd))
	cmd.AddCommand(newDocsCmd(cmd))
	return cmd
}

func newCompletionCmd(root *cobra.Command) *cobra.Command {
	const (
		longDesc = `Generate shell completion scripts for go-docmd.

The output should be evaluated by your shell. For example:

  # bash
  go-docmd completion bash > /usr/local/etc/bash_completion.d/go-docmd

  # zsh
  go-docmd completion zsh > "${fpath[1]}/_go-docmd"

  # fish
  go-docmd completion fish | source

  # PowerShell
  go-docmd completion powershell | Out-String | Invoke-Expression
`
	)
	cmd := &cobra.Command{
		Use:                   "completion [bash|zsh|fish|powershell]",
		Short:                 "Generate shell completion scripts",
		Long:                  longDesc,
		Args:                  cobra.ExactValidArgs(1),
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		SilenceUsage:          true,
		SilenceErrors:         true,
		DisableFlagsInUseLine: true,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return root.GenBashCompletion(cmd.OutOrStdout())
		case "zsh":
			return root.GenZshCompletion(cmd.OutOrStdout())
		case "fish":
			return root.GenFishCompletion(cmd.OutOrStdout(), true)
		case "powershell":
			return root.GenPowerShellCompletion(cmd.OutOrStdout())
		default:
			return fmt.Errorf("unsupported shell %q", args[0])
		}
	}
	return cmd
}

func newDocsCmd(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gen-docs [directory]",
		Short: "Generate Markdown reference docs for the CLI",
		Long: strings.TrimSpace(`
Write a Markdown file per command (suitable for publishing CLI docs).

Example:

  go-docmd gen-docs ./docs/cli
`),
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		target := args[0]
		if target == "" {
			return fmt.Errorf("target directory is required")
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			return err
		}
		return cobradoc.GenMarkdownTree(root, target)
	}
	return cmd
}
