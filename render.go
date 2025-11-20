package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/doc"
	"go/format"
	"go/token"
	"io"
	"sort"
	"strings"
)

type markdownRenderer struct {
	options options
	pkg     *doc.Package
	fileset *token.FileSet
}

func (r *markdownRenderer) renderPackage(w io.Writer) {
	if r.pkg.Name != "main" {
		fmt.Fprintf(w, "# package %s\n\n", r.pkg.Name)
		if r.pkg.ImportPath != "" {
			fmt.Fprintf(w, "`import \"%s\"`\n\n", r.pkg.ImportPath)
		}
	}
	if doc := r.docMarkdown(r.pkg.Doc); doc != "" {
		fmt.Fprintln(w, doc)
		fmt.Fprintln(w)
	}
	if r.pkg.Name == "main" && !r.options.showCmd && !r.options.all {
		r.renderPackageSummary(w)
		if r.options.includeMainVars || r.options.all {
			r.renderValuesSection(w, "Variables", r.pkg.Vars)
		}
		if r.options.includeMainFuncs || r.options.all {
			r.renderFuncsSection(w, "Functions", r.pkg.Funcs, "")
		}
		return
	}
	r.renderPackageSummary(w)
	if r.options.all {
		r.renderValuesSection(w, "Constants", r.pkg.Consts)
		r.renderValuesSection(w, "Variables", r.pkg.Vars)
		r.renderFuncsSection(w, "Functions", r.pkg.Funcs, "")
		r.renderTypesSection(w, r.pkg.Types)
	}
}

func (r *markdownRenderer) renderSymbol(w io.Writer, symbol string) bool {
	var rendered bool
	for _, t := range r.pkg.Types {
		if r.matchName(t.Name, symbol) {
			r.renderTypeDoc(w, t)
			rendered = true
		}
	}
	for _, f := range r.pkg.Funcs {
		if r.matchName(f.Name, symbol) {
			r.renderFuncDoc(w, f, "")
			rendered = true
		}
	}
	for _, v := range r.pkg.Consts {
		if r.valueHasName(v, symbol) {
			r.renderValueDoc(w, v)
			rendered = true
		}
	}
	for _, v := range r.pkg.Vars {
		if r.valueHasName(v, symbol) {
			r.renderValueDoc(w, v)
			rendered = true
		}
	}
	for _, t := range r.pkg.Types {
		for _, v := range t.Consts {
			if r.valueHasName(v, symbol) {
				r.renderValueDoc(w, v)
				rendered = true
			}
		}
		for _, v := range t.Vars {
			if r.valueHasName(v, symbol) {
				r.renderValueDoc(w, v)
				rendered = true
			}
		}
		for _, f := range t.Funcs {
			if r.matchName(f.Name, symbol) {
				r.renderFuncDoc(w, f, t.Name)
				rendered = true
			}
		}
	}
	return rendered
}

func (r *markdownRenderer) renderMethod(w io.Writer, typeName, methodName string) bool {
	var rendered bool
	for _, t := range r.pkg.Types {
		if !r.matchName(t.Name, typeName) {
			continue
		}
		for _, m := range t.Methods {
			if r.matchName(m.Name, methodName) {
				r.renderFuncDoc(w, m, t.Name)
				rendered = true
			}
		}
		if r.renderFieldDoc(w, t, methodName) {
			rendered = true
		}
	}
	return rendered
}

func (r *markdownRenderer) renderPackageSummary(w io.Writer) {
	var entries []string
	for _, v := range r.pkg.Consts {
		entries = append(entries, bulletLine(r.valueTitle(v), r.summaryText(v.Doc)))
	}
	if r.pkg.Name != "main" || r.options.includeMainVars || r.options.all {
		for _, v := range r.pkg.Vars {
			entries = append(entries, bulletLine(r.valueTitle(v), r.summaryText(v.Doc)))
		}
	}
	if r.pkg.Name != "main" || r.options.includeMainFuncs || r.options.all {
		for _, f := range r.pkg.Funcs {
			entries = append(entries, bulletLine(r.signature(f.Decl), r.summaryText(f.Doc)))
		}
	}
	for _, t := range r.pkg.Types {
		entries = append(entries, bulletLine("type "+t.Name, r.summaryText(t.Doc)))
	}
	if len(entries) == 0 {
		return
	}
	sort.Strings(entries)
	for _, entry := range entries {
		fmt.Fprintln(w, entry)
	}
	fmt.Fprintln(w)
}

func (r *markdownRenderer) renderTypesSection(w io.Writer, types []*doc.Type) {
	for _, t := range types {
		r.renderTypeDoc(w, t)
	}
}

func (r *markdownRenderer) renderTypeDoc(w io.Writer, t *doc.Type) {
	fmt.Fprintf(w, "## type %s\n\n", t.Name)
	r.writeCodeBlock(w, r.formatNode(t.Decl))
	if doc := r.docMarkdown(t.Doc); doc != "" {
		fmt.Fprintln(w, doc)
		fmt.Fprintln(w)
	}
	r.renderValuesSection(w, "Constants", t.Consts)
	r.renderValuesSection(w, "Variables", t.Vars)
	r.renderFuncsSection(w, "Functions returning "+t.Name, t.Funcs, "")
	r.renderFuncsSection(w, "Methods", t.Methods, t.Name)
}

func (r *markdownRenderer) renderValuesSection(w io.Writer, title string, values []*doc.Value) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(w, "### %s\n\n", title)
	for _, v := range values {
		r.renderValueDoc(w, v)
	}
	fmt.Fprintln(w)
}

func (r *markdownRenderer) renderValueDoc(w io.Writer, v *doc.Value) {
	if r.options.short {
		fmt.Fprintf(w, "%s\n", bulletLine(r.valueTitle(v), r.summaryText(v.Doc)))
		return
	}
	fmt.Fprintf(w, "#### %s\n\n", r.valueTitle(v))
	r.writeCodeBlock(w, r.formatNode(v.Decl))
	if doc := r.docMarkdown(v.Doc); doc != "" {
		fmt.Fprintln(w, doc)
		fmt.Fprintln(w)
	}
}

func (r *markdownRenderer) renderFuncsSection(w io.Writer, title string, funcs []*doc.Func, receiver string) {
	if len(funcs) == 0 {
		return
	}
	fmt.Fprintf(w, "### %s\n\n", title)
	for _, f := range funcs {
		r.renderFuncDoc(w, f, receiver)
	}
	fmt.Fprintln(w)
}

func (r *markdownRenderer) renderFuncDoc(w io.Writer, f *doc.Func, receiver string) {
	if r.options.short {
		fmt.Fprintf(w, "%s\n", bulletLine(r.signature(f.Decl), r.summaryText(f.Doc)))
		return
	}
	name := f.Name
	if receiver != "" {
		name = receiver + "." + f.Name
	}
	fmt.Fprintf(w, "#### %s\n\n", name)
	if r.options.showSource {
		r.writeCodeBlock(w, r.formatNode(f.Decl))
	} else {
		fmt.Fprintf(w, "```go\n%s\n```\n\n", r.signature(f.Decl))
	}
	if doc := r.docMarkdown(f.Doc); doc != "" {
		fmt.Fprintln(w, doc)
		fmt.Fprintln(w)
	}
}

func (r *markdownRenderer) renderFieldDoc(w io.Writer, t *doc.Type, fieldName string) bool {
	spec := findTypeSpec(t.Decl, t.Name)
	if spec == nil {
		return false
	}
	st, ok := spec.Type.(*ast.StructType)
	if !ok || st.Fields == nil {
		return false
	}
	var rendered bool
	for _, field := range st.Fields.List {
		docText := ""
		if field.Doc != nil {
			docText = field.Doc.Text()
		}
		for _, name := range field.Names {
			if r.matchName(name.Name, fieldName) {
				if r.options.short {
					fmt.Fprintf(w, "%s\n", bulletLine(fmt.Sprintf("%s.%s", t.Name, name.Name), r.summaryText(docText)))
				} else {
					fmt.Fprintf(w, "#### %s.%s\n\n", t.Name, name.Name)
					r.writeCodeBlock(w, r.formatField(field))
					if doc := r.docMarkdown(docText); doc != "" {
						fmt.Fprintln(w, doc)
						fmt.Fprintln(w)
					}
				}
				rendered = true
			}
		}
	}
	return rendered
}

func findTypeSpec(decl *ast.GenDecl, name string) *ast.TypeSpec {
	if decl == nil {
		return nil
	}
	for _, spec := range decl.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		if ts.Name != nil && ts.Name.Name == name {
			return ts
		}
	}
	return nil
}

func (r *markdownRenderer) writeCodeBlock(w io.Writer, code string) {
	if code == "" {
		return
	}
	fmt.Fprintf(w, "```go\n%s\n```\n\n", strings.TrimSpace(code))
}

func (r *markdownRenderer) formatNode(node ast.Node) string {
	if node == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, r.fileset, node); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

func (r *markdownRenderer) formatField(field *ast.Field) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, r.fileset, field); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

func (r *markdownRenderer) signature(decl *ast.FuncDecl) string {
	if decl == nil || decl.Type == nil {
		return ""
	}
	var buf bytes.Buffer
	buf.WriteString("func ")
	if decl.Recv != nil {
		var recv bytes.Buffer
		_ = format.Node(&recv, r.fileset, decl.Recv)
		buf.WriteString("(")
		buf.WriteString(strings.TrimSpace(recv.String()))
		buf.WriteString(") ")
	}
	buf.WriteString(decl.Name.Name)
	var typ bytes.Buffer
	_ = format.Node(&typ, r.fileset, decl.Type)
	sig := typ.String()
	sig = strings.TrimPrefix(sig, "func")
	buf.WriteString(strings.TrimSpace(sig))
	return strings.TrimSpace(buf.String())
}

func (r *markdownRenderer) valueTitle(v *doc.Value) string {
	return strings.Join(v.Names, ", ")
}

func (r *markdownRenderer) docMarkdown(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	return dedentMarkdown(trimmed)
}

func dedentMarkdown(src string) string {
	lines := strings.Split(src, "\n")
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := leadingWhitespace(line)
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent <= 0 {
		return src
	}
	for i, line := range lines {
		if len(line) >= minIndent {
			lines[i] = line[minIndent:]
		}
	}
	return strings.Join(lines, "\n")
}

func leadingWhitespace(line string) int {
	count := 0
	for _, r := range line {
		if r == ' ' || r == '\t' {
			count++
			continue
		}
		break
	}
	return count
}

func (r *markdownRenderer) summaryText(text string) string {
	md := r.docMarkdown(text)
	if md == "" {
		return ""
	}
	md = strings.ReplaceAll(md, "\n", " ")
	if idx := strings.Index(md, ". "); idx >= 0 {
		return strings.TrimSpace(md[:idx+1])
	}
	return strings.TrimSpace(md)
}

func (r *markdownRenderer) packageSummary() string {
	return r.summaryText(r.pkg.Doc)
}

func (r *markdownRenderer) matchName(name, target string) bool {
	if r.options.caseSensitive {
		return name == target
	}
	return strings.EqualFold(name, target)
}

func (r *markdownRenderer) valueHasName(v *doc.Value, name string) bool {
	for _, n := range v.Names {
		if r.matchName(n, name) {
			return true
		}
	}
	return false
}

func bulletLine(signature, summary string) string {
	if summary == "" {
		return fmt.Sprintf("- `%s`", signature)
	}
	return fmt.Sprintf("- `%s` — %s", signature, summary)
}
