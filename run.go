package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/doc"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/tools/go/packages"
)

type options struct {
	all              bool
	caseSensitive    bool
	showCmd          bool
	short            bool
	showSource       bool
	unexported       bool
	outputPath       string
	inplace          bool
	includeMainVars  bool
	includeMainFuncs bool
}

type invocation struct {
	pkgExpr string
	symbol  string
	method  string
}

type docResult struct {
	Markdown []byte
	Summary  string
}

type cliApp struct {
	stdout io.Writer
	opts   options
}

func run(argv []string, stdout io.Writer) error {
	cmd := newRootCmd(stdout)
	cmd.SetArgs(normalizeLegacyArgs(argv))
	return cmd.Execute()
}

func (app *cliApp) execute(ctx context.Context, positionals []string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	opts := app.opts
	if opts.inplace && opts.outputPath != "" {
		return errors.New("-o cannot be combined with -inplace")
	}
	if opts.inplace {
		if len(positionals) > 1 {
			return errors.New("in-place mode accepts at most one package argument")
		}
		root := "."
		if len(positionals) == 1 {
			root = positionals[0]
		}
		return documentPackageTree(ctx, root, opts)
	}
	if wantsDirectoryOutput(opts.outputPath) {
		if len(positionals) > 1 {
			return errors.New("directory output accepts at most one package argument")
		}
		root := "."
		if len(positionals) == 1 {
			root = positionals[0]
		}
		return documentPackageTree(ctx, root, opts)
	}
	if opts.all && len(positionals) > 1 {
		return errors.New("-all can only be used with a single package argument")
	}
	candidates, err := buildCandidates(positionals)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return errors.New("no arguments provided")
	}

	var lastErr error
	for _, cand := range candidates {
		pkgInfo, err := resolvePackage(ctx, cand.pkgExpr)
		if err != nil {
			lastErr = err
			continue
		}
		result, handled, err := documentTarget(pkgInfo, cand.symbol, cand.method, opts)
		if err != nil {
			return err
		}
		if !handled {
			lastErr = fmt.Errorf("no matching symbol %q in %s", displaySymbol(cand.symbol, cand.method), pkgInfo.PkgPath)
			continue
		}
		return writeOutput(opts.outputPath, app.stdout, result.Markdown)
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("unable to locate documentation target")
}

func displaySymbol(symbol, method string) string {
	if symbol == "" {
		return ""
	}
	if method == "" {
		return symbol
	}
	return symbol + "." + method
}

func writeOutput(path string, stdout io.Writer, data []byte) error {
	if path == "" || path == "-" {
		_, err := stdout.Write(data)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

var legacyLongFlagSet = map[string]struct{}{
	"all":            {},
	"cmd":            {},
	"short":          {},
	"src":            {},
	"inplace":        {},
	"mainvars":       {},
	"mainfuncs":      {},
	"output":         {},
	"case-sensitive": {},
}

func normalizeLegacyArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	modified := false
	converted := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			converted = append(converted, arg)
			converted = append(converted, args[i+1:]...)
			if i != len(args)-1 {
				modified = true
			}
			break
		}
		if !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") || arg == "-" {
			converted = append(converted, arg)
			continue
		}
		if len(arg) == 2 {
			converted = append(converted, arg)
			continue
		}
		if idx := strings.Index(arg, "="); idx > 0 {
			name := arg[1:idx]
			if _, ok := legacyLongFlagSet[name]; ok {
				converted = append(converted, "--"+name+arg[idx:])
				modified = true
				continue
			}
		}
		name := arg[1:]
		if _, ok := legacyLongFlagSet[name]; ok {
			converted = append(converted, "--"+name)
			modified = true
			continue
		}
		converted = append(converted, arg)
	}
	if !modified && len(converted) == len(args) {
		return args
	}
	return converted
}

func buildCandidates(args []string) ([]invocation, error) {
	switch len(args) {
	case 0:
		return []invocation{{pkgExpr: "."}}, nil
	case 1:
		return singleArgCandidates(args[0]), nil
	case 2:
		symbol, method := splitSymbol(args[1])
		return []invocation{{pkgExpr: args[0], symbol: symbol, method: method}}, nil
	default:
		return nil, errors.New("too many positional arguments")
	}
}

func singleArgCandidates(arg string) []invocation {
	seen := make(map[string]struct{})
	var candidates []invocation
	add := func(pkgExpr, symbol, method string) {
		key := pkgExpr + "|" + symbol + "|" + method
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		cand := invocation{
			pkgExpr: pkgExpr,
			symbol:  symbol,
			method:  method,
		}
		if cand.pkgExpr == "" {
			cand.pkgExpr = "."
		}
		candidates = append(candidates, cand)
	}

	if startsWithUpper(arg) && !strings.ContainsAny(arg, `/\`) {
		// Force local symbol lookup first.
		symbol, method := splitSymbol(arg)
		add(".", symbol, method)
	}

	// Treat as package path (no symbol).
	add(arg, "", "")

	if strings.Contains(arg, ".") {
		for _, cand := range parseCompound(arg) {
			add(cand.pkgExpr, cand.symbol, cand.method)
		}
	}

	// Finally, treat as local symbol if not already present.
	symbol, method := splitSymbol(arg)
	add(".", symbol, method)

	return candidates
}

func parseCompound(arg string) []invocation {
	var result []invocation
	for i := 0; i < len(arg); i++ {
		if arg[i] != '.' {
			continue
		}
		pkgExpr := arg[:i]
		symbolSpec := arg[i+1:]
		if pkgExpr == "" {
			continue
		}
		symbol, method := splitSymbol(symbolSpec)
		result = append(result, invocation{
			pkgExpr: pkgExpr,
			symbol:  symbol,
			method:  method,
		})
	}
	return result
}

func splitSymbol(spec string) (string, string) {
	if spec == "" {
		return "", ""
	}
	parts := strings.Split(spec, ".")
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], ".")
}

func startsWithUpper(s string) bool {
	if s == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsUpper(r)
}

func documentTarget(pkgInfo *packages.Package, symbol, method string, opts options) (docResult, bool, error) {
	docPkg, err := buildDocPackage(pkgInfo, opts)
	if err != nil {
		return docResult{}, false, err
	}
	var buf bytes.Buffer
	renderer := markdownRenderer{
		options: opts,
		pkg:     docPkg,
		fileset: pkgInfo.Fset,
	}
	switch {
	case symbol == "":
		renderer.renderPackage(&buf)
		return docResult{
			Markdown: buf.Bytes(),
			Summary:  renderer.packageSummary(),
		}, true, nil
	case method == "":
		ok := renderer.renderSymbol(&buf, symbol)
		return docResult{Markdown: buf.Bytes()}, ok, nil
	default:
		ok := renderer.renderMethod(&buf, symbol, method)
		return docResult{Markdown: buf.Bytes()}, ok, nil
	}
}

func buildDocPackage(pkgInfo *packages.Package, opts options) (*doc.Package, error) {
	mode := doc.Mode(0)
	if opts.unexported || opts.all {
		mode |= doc.AllDecls | doc.AllMethods
	}
	if opts.showSource {
		mode |= doc.PreserveAST
	}
	return doc.NewFromFiles(pkgInfo.Fset, pkgInfo.Syntax, pkgInfo.PkgPath, mode)
}

func loadPackage(ctx context.Context, pattern string) (*packages.Package, error) {
	cfg := &packages.Config{
		Context: ctx,
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedFiles |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedTypesSizes | packages.NeedModule | packages.NeedImports,
	}
	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		return nil, err
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no Go packages matched %q", pattern)
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		return nil, fmt.Errorf("%s", pkg.Errors[0])
	}
	return pkg, nil
}

func resolvePackage(ctx context.Context, expr string) (*packages.Package, error) {
	try := []string{expr}
	if expr == "" {
		try = []string{"."}
	}
	for _, candidate := range try {
		if candidate == "" {
			continue
		}
		if pkg, err := loadPackage(ctx, candidate); err == nil {
			return pkg, nil
		}
	}
	if match := matchStdSuffix(expr); match != "" {
		return loadPackage(ctx, match)
	}
	return nil, fmt.Errorf("could not resolve package path for %q", expr)
}

var (
	stdOnce     sync.Once
	stdPackages []string
	stdErr      error
)

func loadStdPackages() {
	cfg := &packages.Config{
		Mode: packages.NeedName,
	}
	pkgs, err := packages.Load(cfg, "std", "cmd/...")
	if err != nil {
		stdErr = err
		return
	}
	for _, pkg := range pkgs {
		stdPackages = append(stdPackages, pkg.PkgPath)
	}
	sort.Strings(stdPackages)
}

func matchStdSuffix(arg string) string {
	if arg == "" {
		return ""
	}
	stdOnce.Do(loadStdPackages)
	if stdErr != nil {
		return ""
	}
	var best string
	for _, path := range stdPackages {
		if path == arg || strings.HasSuffix(path, "/"+arg) {
			if best == "" || path < best {
				best = path
			}
		}
	}
	return best
}

func wantsDirectoryOutput(path string) bool {
	if path == "" || path == "-" {
		return false
	}
	info, err := os.Stat(path)
	if err == nil {
		return info.IsDir()
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false
	}
	if strings.HasSuffix(path, string(os.PathSeparator)) {
		return true
	}
	return filepath.Ext(path) == ""
}

func documentPackageTree(ctx context.Context, root string, opts options) error {
	docs, baseDir, err := collectPackageDocs(ctx, root, opts)
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return fmt.Errorf("no packages matched %q", root)
	}
	if opts.inplace {
		if baseDir == "" {
			return errors.New("cannot determine base directory for in-place output")
		}
		return writePackageDocsInPlace(baseDir, docs)
	}
	if opts.outputPath == "" {
		return errors.New("directory output requires -o pointing to a directory")
	}
	return writePackageDocsToDir(opts.outputPath, docs)
}

func collectPackageDocs(ctx context.Context, root string, opts options) ([]treeDoc, string, error) {
	pkgs, err := loadPackageTree(ctx, root)
	if err != nil {
		return nil, "", err
	}
	if len(pkgs) == 0 {
		return nil, "", nil
	}
	baseDir := resolveBaseDir(root)
	docs := make([]treeDoc, 0, len(pkgs))
	for _, pkgInfo := range pkgs {
		docRes, handled, err := documentTarget(pkgInfo, "", "", opts)
		if err != nil {
			return nil, "", err
		}
		if !handled {
			continue
		}
		pkgDir := absolutePath(packageDir(pkgInfo))
		if baseDir == "" && pkgDir != "" {
			baseDir = pkgDir
		}
		relDir := deriveRelativeDir(pkgInfo, baseDir, pkgDir)
		docs = append(docs, treeDoc{
			relDir:   relDir,
			pkgDir:   pkgDir,
			pkgPath:  pkgInfo.PkgPath,
			summary:  docRes.Summary,
			markdown: docRes.Markdown,
		})
	}
	return docs, baseDir, nil
}

func absolutePath(dir string) string {
	if dir == "" {
		return ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return filepath.Clean(dir)
	}
	return abs
}

func deriveRelativeDir(pkg *packages.Package, baseDir, pkgDir string) string {
	if baseDir != "" && pkgDir != "" {
		if rel, err := filepath.Rel(baseDir, pkgDir); err == nil && rel != "" && !strings.HasPrefix(rel, "..") {
			if rel == "." {
				return "."
			}
			return filepath.ToSlash(rel)
		}
	}
	if pkg.PkgPath != "" {
		return filepath.FromSlash(pkg.PkgPath)
	}
	if pkgDir != "" {
		return filepath.Base(pkgDir)
	}
	return pkg.Name
}

func loadPackageTree(ctx context.Context, root string) ([]*packages.Package, error) {
	patterns := buildPatterns(root)
	cfg := &packages.Config{
		Context: ctx,
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedFiles |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedTypesSizes | packages.NeedModule | packages.NeedImports,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}
	unique := make(map[string]*packages.Package)
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			return nil, fmt.Errorf("%s", pkg.Errors[0])
		}
		key := pkg.PkgPath
		if key == "" {
			key = packageDir(pkg)
		}
		unique[key] = pkg
	}
	result := make([]*packages.Package, 0, len(unique))
	for _, pkg := range unique {
		result = append(result, pkg)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].PkgPath < result[j].PkgPath
	})
	return result, nil
}

func buildPatterns(root string) []string {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	root = filepath.ToSlash(root)
	patterns := []string{root}
	if !strings.Contains(root, "...") {
		recursive := root
		if recursive == "." {
			recursive = "./..."
		} else if strings.HasSuffix(recursive, "/") {
			recursive = recursive + "..."
		} else {
			recursive = recursive + "/..."
		}
		patterns = append(patterns, recursive)
	}
	return patterns
}

type treeDoc struct {
	relDir   string
	pkgDir   string
	pkgPath  string
	summary  string
	markdown []byte
}

type tocEntry struct {
	title   string
	link    string
	summary string
}

func writePackageDocsToDir(outDir string, docs []treeDoc) error {
	if outDir == "" {
		return errors.New("missing output directory")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	sort.Slice(docs, func(i, j int) bool {
		return docs[i].relDir < docs[j].relDir
	})
	var entries []tocEntry
	var rootDoc *treeDoc
	var rootPath string
	for i := range docs {
		doc := &docs[i]
		targetDir := outDir
		if doc.relDir != "" && doc.relDir != "." {
			targetDir = filepath.Join(outDir, doc.relDir)
		}
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return err
		}
		filePath := filepath.Join(targetDir, "README.md")
		if doc.relDir == "" || doc.relDir == "." {
			rootDoc = doc
			rootPath = filePath
			continue
		}
		if err := os.WriteFile(filePath, doc.markdown, 0o644); err != nil {
			return err
		}
		entries = append(entries, tocEntry{
			title:   linkTitle(doc),
			link:    filepath.ToSlash(filepath.Join(doc.relDir, "README.md")),
			summary: strings.TrimSpace(doc.summary),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].title < entries[j].title
	})
	toc := buildTOC(entries)
	switch {
	case rootDoc != nil:
		content := appendTOCAfterDoc(rootDoc.markdown, toc)
		if err := os.WriteFile(rootPath, content, 0o644); err != nil {
			return err
		}
	case len(toc) > 0:
		if err := os.WriteFile(filepath.Join(outDir, "README.md"), toc, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func writePackageDocsInPlace(baseDir string, docs []treeDoc) error {
	if baseDir == "" {
		return errors.New("missing base directory for in-place output")
	}
	baseDir = filepath.Clean(baseDir)
	var entries []tocEntry
	var rootDoc *treeDoc
	rootPath := filepath.Join(baseDir, "README.md")
	for i := range docs {
		doc := &docs[i]
		pkgDir := doc.pkgDir
		if pkgDir == "" {
			continue
		}
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			return err
		}
		target := filepath.Join(pkgDir, "README.md")
		if sameDir(pkgDir, baseDir) {
			rootDoc = doc
			rootPath = target
			continue
		}
		if err := os.WriteFile(target, doc.markdown, 0o644); err != nil {
			return err
		}
		relLink, err := filepath.Rel(baseDir, target)
		if err != nil {
			relLink = target
		}
		entries = append(entries, tocEntry{
			title:   linkTitle(doc),
			link:    filepath.ToSlash(relLink),
			summary: strings.TrimSpace(doc.summary),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].title < entries[j].title
	})
	toc := buildTOC(entries)
	var content []byte
	switch {
	case rootDoc != nil:
		content = appendTOCAfterDoc(rootDoc.markdown, toc)
	case len(toc) > 0:
		content = toc
	}
	if len(content) == 0 && rootDoc != nil {
		content = rootDoc.markdown
	}
	if len(content) == 0 {
		return nil
	}
	return os.WriteFile(rootPath, content, 0o644)
}

func sameDir(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func appendTOCAfterDoc(doc []byte, toc []byte) []byte {
	if len(toc) == 0 {
		return append([]byte{}, doc...)
	}
	if len(doc) == 0 {
		return append([]byte{}, toc...)
	}
	content := append([]byte{}, doc...)
	if !bytes.HasSuffix(content, []byte("\n\n")) {
		if bytes.HasSuffix(content, []byte("\n")) {
			content = append(content, '\n')
		} else {
			content = append(content, '\n', '\n')
		}
	}
	content = append(content, toc...)
	return content
}

func linkTitle(doc *treeDoc) string {
	if doc.relDir != "" && doc.relDir != "." {
		return filepath.ToSlash(doc.relDir)
	}
	if doc.pkgPath != "" {
		return doc.pkgPath
	}
	return "."
}

func buildTOC(entries []tocEntry) []byte {
	if len(entries) == 0 {
		return nil
	}
	var buf bytes.Buffer
	buf.WriteString("## Packages\n\n")
	for _, entry := range entries {
		if entry.summary != "" {
			fmt.Fprintf(&buf, "- [%s](%s) — %s\n", entry.title, entry.link, entry.summary)
		} else {
			fmt.Fprintf(&buf, "- [%s](%s)\n", entry.title, entry.link)
		}
	}
	buf.WriteString("\n")
	return buf.Bytes()
}

func resolveBaseDir(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	root = strings.TrimSuffix(root, "/...")
	root = strings.TrimSuffix(root, "\\...")
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return ""
	}
	base, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	return base
}

func packageDir(pkg *packages.Package) string {
	if len(pkg.GoFiles) > 0 {
		return filepath.Dir(pkg.GoFiles[0])
	}
	if len(pkg.CompiledGoFiles) > 0 {
		return filepath.Dir(pkg.CompiledGoFiles[0])
	}
	return ""
}
