package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageMarkdown(t *testing.T) {
	var buf bytes.Buffer
	if err := run([]string{"-mainvars", "-mainfuncs", "./testdata/example"}, &buf); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := buf.String()
	assertContains(t, out, "# package example")
	assertContains(t, out, "Package example demonstrates documentation rendering")
	assertContains(t, out, "`Answer`")
	assertContains(t, out, "`type Greeter`")
	assertContains(t, out, "**Alpha**: demonstrates bold formatting preservation.")
}

func TestSymbolMarkdown(t *testing.T) {
	var buf bytes.Buffer
	if err := run([]string{"./testdata/example.Greeter"}, &buf); err != nil {
		t.Fatalf("run: %v", err)
	}
	assertContains(t, buf.String(), "## type Greeter")
}

func TestMethodMarkdown(t *testing.T) {
	var buf bytes.Buffer
	if err := run([]string{"./testdata/example.Greeter.Greet"}, &buf); err != nil {
		t.Fatalf("run: %v", err)
	}
	assertContains(t, buf.String(), "#### Greeter.Greet")
}

func TestOutputFlagWritesFile(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "out.md")
	if err := run([]string{"-o", target, "./testdata/example.Greeter"}, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	assertContains(t, string(content), "type Greeter")
}

func TestDirectoryOutputWritesTree(t *testing.T) {
	tmp := t.TempDir()
	if err := run([]string{"-mainvars", "-mainfuncs", "-o", tmp, "./testdata/example"}, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}
	root := filepath.Join(tmp, "README.md")
	content, err := os.ReadFile(root)
	if err != nil {
		t.Fatalf("read root: %v", err)
	}
	rootStr := string(content)
	assertContains(t, rootStr, "# package example")
	assertContains(t, rootStr, "## Packages")
	assertContains(t, rootStr, "[subpkg](subpkg/README.md)")
	assertContains(t, rootStr, "**Alpha**: demonstrates bold formatting preservation.")
	assertTOCAfterDoc(t, rootStr, "# package example", "## Packages")
	subContent, err := os.ReadFile(filepath.Join(tmp, "subpkg", "README.md"))
	if err != nil {
		t.Fatalf("read subpkg: %v", err)
	}
	assertContains(t, string(subContent), "# package subpkg")
	assertContains(t, string(subContent), "Message exposes a sample constant")
}

func TestInPlaceModeWritesPackageReadmes(t *testing.T) {
	rootPattern := "./testdata/example"
	rootDir := filepath.Clean(rootPattern)
	rootReadme := filepath.Join(rootDir, "README.md")
	subReadme := filepath.Join(rootDir, "subpkg", "README.md")
	cleanup := func() {
		_ = os.Remove(rootReadme)
		_ = os.Remove(subReadme)
	}
	cleanup()
	t.Cleanup(cleanup)
	if err := run([]string{"-mainvars", "-mainfuncs", "-inplace", rootPattern}, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}
	rootContent, err := os.ReadFile(rootReadme)
	if err != nil {
		t.Fatalf("read root: %v", err)
	}
	rootStr := string(rootContent)
	assertContains(t, rootStr, "# package example")
	assertContains(t, rootStr, "## Packages")
	assertContains(t, rootStr, "subpkg/README.md")
	assertTOCAfterDoc(t, rootStr, "# package example", "## Packages")
	subContent, err := os.ReadFile(subReadme)
	if err != nil {
		t.Fatalf("read subpkg readme: %v", err)
	}
	assertContains(t, string(subContent), "# package subpkg")
	assertContains(t, string(subContent), "Message exposes a sample constant")
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected output to contain %q\n\n%s", needle, haystack)
	}
}

func assertTOCAfterDoc(t *testing.T, text, docHeading, tocHeading string) {
	t.Helper()
	docIdx := strings.Index(text, docHeading)
	tocIdx := strings.Index(text, tocHeading)
	if docIdx == -1 || tocIdx == -1 {
		t.Fatalf("missing doc heading %q or toc heading %q", docHeading, tocHeading)
	}
	if tocIdx <= docIdx {
		t.Fatalf("expected %q to appear after %q\n\n%s", tocHeading, docHeading, text)
	}
}

func TestHelpFlag(t *testing.T) {
	var buf bytes.Buffer
	if err := run([]string{"--help"}, &buf); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := buf.String()
	assertContains(t, out, "go-docmd [flags] [package|[package.]symbol[.method]]")
	assertContains(t, out, "--all")
	assertContains(t, out, "completion  Generate shell completion scripts")
}

func TestCompletionCommand(t *testing.T) {
	var buf bytes.Buffer
	if err := run([]string{"completion", "bash"}, &buf); err != nil {
		t.Fatalf("run: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatalf("expected completion output")
	}
	assertContains(t, buf.String(), "__start_go-docmd")
}

func TestGenDocsCommand(t *testing.T) {
	tmp := t.TempDir()
	if err := run([]string{"gen-docs", tmp}, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}
	files, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected CLI docs to be written")
	}
	var foundRoot bool
	for _, f := range files {
		if f.Name() == "go-docmd.md" {
			foundRoot = true
			break
		}
	}
	if !foundRoot {
		t.Fatalf("expected go-docmd.md in docs output, got %v", files)
	}
}
