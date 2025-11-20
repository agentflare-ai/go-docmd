// # go-docmd
//
// `go-docmd` is a drop-in companion to `go doc` that emits Markdown instead of
// plaintext. It uses the standard library's `go/doc` and `go/doc/comment`
// packages to parse Go documentation comments and renders them as Markdown that
// works well on GitHub.
//
// Key capabilities:
//
//   - mirror `go doc` argument parsing so you can inspect packages, symbols,
//     and methods via patterns like `pkg`, `pkg.Type`, or `pkg.Type.Method`.
//   - replicate the commonly used `go doc` flags (`-all`, `-cmd`, `-short`,
//     `-src`, `-u`, etc.).
//   - render Markdown either to stdout (default) or any file path via `-o`.
//   - export documentation for entire module trees (`./...`) and emit one
//     `README.md` per package, optionally in-place.
//   - provide `-mainvars` and `-mainfuncs` so command packages can opt into the
//     variable/function tables that are hidden by default.
//   - ship a Cobra-powered CLI with rich `--help`, `--version`, shell completion,
//     and a `gen-docs` helper for publishing the CLI reference itself.
//
// ## Usage
//
//	go run ./go-docmd [flags] [package|[package.]symbol[.method]]
//
// Examples:
//
//   - Render the current package and print to stdout:
//
//     go run ./go-docmd
//
//   - Export docs for a package tree into a docs folder:
//
//     go run ./go-docmd -cmd -all -o ./agentmlx/docs ./agentmlx/...
//
//   - Update package READMEs in place (dogfooding):
//
//     go run ./go-docmd -cmd -all -inplace ./af-proxy
//
//   - Install shell completion for bash (similar invocations exist for zsh, fish,
//     and PowerShell):
//
//     go run ./go-docmd completion bash > /usr/local/etc/bash_completion.d/go-docmd
//
// ## Supported Flags
//
// The CLI mirrors `go doc` and extends it with Markdown-specific behavior:
//
//   - `-all`: show all documentation for the package, including unexported
//     declarations (same as `go doc -all`).
//   - `-c`: make symbol matching case-sensitive.
//   - `-cmd`: include symbol documentation for `package main`.
//   - `-short`: collapse each symbol to a single-line summary.
//   - `-src`: include the full declaration source.
//   - `-u`: include unexported symbols.
//   - `-o FILE`: write Markdown to `FILE` (stdout when omitted).
//   - `-inplace`: treat the output path as a directory and write one
//     `README.md` into each package directory (overwriting existing files).
//   - `-mainvars`: show package-level variables for `package main` (default:
//     hidden so command docs stay concise).
//   - `-mainfuncs`: show package-level functions for `package main`.
//
// ## Shell Completion
//
// Autocompletion is provided via Cobra's generators:
//
//	go run ./go-docmd completion bash        # bash
//	go run ./go-docmd completion zsh         # zsh
//	go run ./go-docmd completion fish | source
//	go run ./go-docmd completion powershell | Out-String | Invoke-Expression
//
// Add the appropriate command to your shell startup files (see Cobra's docs for
// installation paths) and enjoy tab-completion for flags, subcommands, and Go
// package arguments.
//
// ## CLI Docs
//
// `go-docmd` can generate Markdown for each CLI command via `gen-docs`. This is
// handy when you want to publish CLI reference docs alongside the rest of your
// project documentation:
//
//	go run ./go-docmd gen-docs ./docs/cli
//
// Every command becomes its own Markdown file under the provided directory.
//
// ## Directory Mode
//
// When `-o` points to a directory (or has no extension) the tool walks the
// provided package pattern, generates documentation for every discovered
// package, and writes a `README.md` per package under that directory. The root
// README automatically includes a table of contents linking to each
// subpackage's README.
//
// ## In-Place Mode
//
// `-inplace` behaves like directory mode except output is written directly into
// the source tree. This is useful when you want each package directory to
// contain an always-up-to-date `README.md`, and it's how we keep this file
// in sync.
//
// ## Dogfooding the README
//
// This repository generates its own README via:
//
//	go run . -cmd -o README.md .
//
// CI runs the command above and fails if the README does not match the
// generated output, so documentation changes must flow through `go-docmd`
// itself.
package main
