package service

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ASTAnalyzer struct {
	repoRoots []string
}

type FileAnalysis struct {
	Path       string              `json:"path"`
	Package    string              `json:"package"`
	Doc        string              `json:"doc,omitempty"`
	Imports    []string            `json:"imports"`
	Functions  []FunctionAnalysis  `json:"functions"`
	Structs    []StructAnalysis    `json:"structs"`
	Interfaces []InterfaceAnalysis `json:"interfaces"`
}

type FunctionAnalysis struct {
	Name    string   `json:"name"`
	Params  []string `json:"params"`
	Returns []string `json:"returns"`
	Doc     string   `json:"doc,omitempty"`
}

type StructAnalysis struct {
	Name   string          `json:"name"`
	Fields []FieldAnalysis `json:"fields"`
	Doc    string          `json:"doc,omitempty"`
}

type InterfaceAnalysis struct {
	Name    string           `json:"name"`
	Methods []MethodAnalysis `json:"methods"`
	Doc     string           `json:"doc,omitempty"`
}

type MethodAnalysis struct {
	Name    string   `json:"name"`
	Params  []string `json:"params"`
	Returns []string `json:"returns"`
}

type FieldAnalysis struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Tag  string `json:"tag,omitempty"`
}

type PackageAnalysis struct {
	PackagePath         string              `json:"package_path"`
	Files               []FileAnalysis      `json:"files"`
	PackageDependencies []string            `json:"package_dependencies"`
	ProjectDependencies map[string][]string `json:"project_dependencies"`
}

func NewASTAnalyzer(repoRoots []string) *ASTAnalyzer {
	return &ASTAnalyzer{repoRoots: canonicalRoots(repoRoots)}
}

func canonicalRoots(roots []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		trimmed := strings.TrimSpace(root)
		if trimmed == "" {
			continue
		}
		abs, err := filepath.Abs(trimmed)
		if err != nil {
			continue
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		out = append(out, abs)
	}
	if len(out) == 0 {
		if wd, err := os.Getwd(); err == nil {
			out = append(out, wd)
		}
	}
	return out
}

func (a *ASTAnalyzer) AnalyzeFile(path string) (*FileAnalysis, error) {
	absPath, root, err := a.resolveWithinRoots(path)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, absPath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	analysis := &FileAnalysis{
		Path:       absPath,
		Package:    f.Name.Name,
		Doc:        commentText(f.Doc),
		Imports:    importsFromFile(f),
		Functions:  make([]FunctionAnalysis, 0),
		Structs:    make([]StructAnalysis, 0),
		Interfaces: make([]InterfaceAnalysis, 0),
	}

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			analysis.Functions = append(analysis.Functions, FunctionAnalysis{
				Name:    d.Name.Name,
				Params:  fieldsToTypedStrings(fset, d.Type.Params),
				Returns: fieldsToTypedStrings(fset, d.Type.Results),
				Doc:     commentText(d.Doc),
			})
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				switch t := ts.Type.(type) {
				case *ast.StructType:
					analysis.Structs = append(analysis.Structs, StructAnalysis{
						Name:   ts.Name.Name,
						Fields: structFields(fset, t.Fields),
						Doc:    firstNonEmpty(commentText(ts.Doc), commentText(d.Doc)),
					})
				case *ast.InterfaceType:
					analysis.Interfaces = append(analysis.Interfaces, InterfaceAnalysis{
						Name:    ts.Name.Name,
						Methods: interfaceMethods(fset, t.Methods),
						Doc:     firstNonEmpty(commentText(ts.Doc), commentText(d.Doc)),
					})
				}
			}
		}
	}

	if root != "" {
		analysis.Path = absPath
	}
	return analysis, nil
}

func (a *ASTAnalyzer) AnalyzePackage(pkgPath string) (*PackageAnalysis, error) {
	absPkgPath, root, err := a.resolveWithinRoots(pkgPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(absPkgPath)
	if err != nil {
		return nil, err
	}

	files := make([]FileAnalysis, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".go") {
			continue
		}
		fa, err := a.AnalyzeFile(filepath.Join(absPkgPath, entry.Name()))
		if err != nil {
			continue
		}
		files = append(files, *fa)
	}

	projectDeps, err := a.projectDependencyMap(root)
	if err != nil {
		return nil, err
	}

	relPkg, _ := filepath.Rel(root, absPkgPath)
	relPkg = filepath.ToSlash(relPkg)
	if relPkg == "." {
		relPkg = ""
	}

	packageDeps := projectDeps[relPkg]
	if packageDeps == nil {
		packageDeps = []string{}
	}

	return &PackageAnalysis{
		PackagePath:         absPkgPath,
		Files:               files,
		PackageDependencies: packageDeps,
		ProjectDependencies: projectDeps,
	}, nil
}

func (a *ASTAnalyzer) projectDependencyMap(root string) (map[string][]string, error) {
	pkgSet := make(map[string]struct{})
	importsByPkg := make(map[string][]string)

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".go") {
			return nil
		}

		relDir, _ := filepath.Rel(root, filepath.Dir(path))
		relDir = filepath.ToSlash(relDir)
		if relDir == "." {
			relDir = ""
		}
		pkgSet[relDir] = struct{}{}

		fa, parseErr := a.AnalyzeFile(path)
		if parseErr != nil {
			return nil
		}
		importsByPkg[relDir] = append(importsByPkg[relDir], fa.Imports...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	pkgPaths := make([]string, 0, len(pkgSet))
	for pkg := range pkgSet {
		pkgPaths = append(pkgPaths, pkg)
	}

	deps := make(map[string][]string)
	for pkg, imports := range importsByPkg {
		deps[pkg] = localDeps(imports, pkgPaths)
	}
	for _, pkg := range pkgPaths {
		if _, ok := deps[pkg]; !ok {
			deps[pkg] = []string{}
		}
	}
	return deps, nil
}

func localDeps(imports, pkgPaths []string) []string {
	set := make(map[string]struct{})
	for _, imp := range imports {
		for _, pkg := range pkgPaths {
			if pkg == "" {
				continue
			}
			if imp == pkg || strings.HasSuffix(imp, "/"+pkg) {
				set[pkg] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(set))
	for dep := range set {
		out = append(out, dep)
	}
	sort.Strings(out)
	return out
}

func importsFromFile(f *ast.File) []string {
	out := make([]string, 0, len(f.Imports))
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, "\"")
		if path != "" {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

func fieldsToTypedStrings(fset *token.FileSet, list *ast.FieldList) []string {
	if list == nil || len(list.List) == 0 {
		return nil
	}
	out := make([]string, 0, len(list.List))
	for _, field := range list.List {
		typ := exprToString(fset, field.Type)
		if len(field.Names) == 0 {
			out = append(out, typ)
			continue
		}
		for _, name := range field.Names {
			out = append(out, name.Name+" "+typ)
		}
	}
	return out
}

func structFields(fset *token.FileSet, fields *ast.FieldList) []FieldAnalysis {
	if fields == nil || len(fields.List) == 0 {
		return nil
	}
	out := make([]FieldAnalysis, 0, len(fields.List))
	for _, field := range fields.List {
		typ := exprToString(fset, field.Type)
		tag := ""
		if field.Tag != nil {
			tag = strings.Trim(field.Tag.Value, "`")
		}
		if len(field.Names) == 0 {
			out = append(out, FieldAnalysis{Name: typ, Type: typ, Tag: tag})
			continue
		}
		for _, name := range field.Names {
			out = append(out, FieldAnalysis{Name: name.Name, Type: typ, Tag: tag})
		}
	}
	return out
}

func interfaceMethods(fset *token.FileSet, fields *ast.FieldList) []MethodAnalysis {
	if fields == nil || len(fields.List) == 0 {
		return nil
	}
	out := make([]MethodAnalysis, 0, len(fields.List))
	for _, field := range fields.List {
		ft, ok := field.Type.(*ast.FuncType)
		if !ok {
			continue
		}
		if len(field.Names) == 0 {
			out = append(out, MethodAnalysis{
				Name:    exprToString(fset, field.Type),
				Params:  fieldsToTypedStrings(fset, ft.Params),
				Returns: fieldsToTypedStrings(fset, ft.Results),
			})
			continue
		}
		for _, name := range field.Names {
			out = append(out, MethodAnalysis{
				Name:    name.Name,
				Params:  fieldsToTypedStrings(fset, ft.Params),
				Returns: fieldsToTypedStrings(fset, ft.Results),
			})
		}
	}
	return out
}

func exprToString(fset *token.FileSet, expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	var b strings.Builder
	if err := printer.Fprint(&b, fset, expr); err != nil {
		return ""
	}
	return b.String()
}

func commentText(group *ast.CommentGroup) string {
	if group == nil {
		return ""
	}
	return strings.TrimSpace(group.Text())
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (a *ASTAnalyzer) resolveWithinRoots(path string) (string, string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	for _, root := range a.repoRoots {
		if absPath == root || strings.HasPrefix(absPath, root+string(os.PathSeparator)) {
			return absPath, root, nil
		}
	}
	return "", "", fmt.Errorf("path fuera de REPO_ROOT: %s", absPath)
}
