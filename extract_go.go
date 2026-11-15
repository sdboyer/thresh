package main

// Go extractors, via go/parser. Stdlib only; the target need not build.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

type goFunc struct {
	Name string
	Recv string // receiver type name, "" for plain functions
	Line int
}

// goFuncs lists the functions and methods declared in a Go source.
func goFuncs(src string) []goFunc {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		return nil
	}
	var out []goFunc
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		g := goFunc{Name: fn.Name.Name, Line: fset.Position(fn.Pos()).Line}
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			g.Recv = typeName(fn.Recv.List[0].Type)
		}
		out = append(out, g)
	}
	return out
}

type goSite struct {
	Type          string
	EnclosingFunc string
	Line          int
}

// goConstructions finds constructions of typeName — composite literals (T{} or
// &T{}) and new(T), generic instantiations (T[X]{}, new(T[X])) included — inside
// function bodies, with the enclosing function. Package-level occurrences (e.g.
// `var _ I = &T{}` interface assertions) are deliberately not constructions.
//
// A bare typeName (T) matches only the unqualified T{}. A qualified one (pkg.T)
// matches x.T{} where x is the package pkg: imported without an alias (so x is
// its name), or under an alias whose import path's last element (less a /vN
// suffix) is pkg. It also matches a bare T{} in a file declaring package pkg.
func goConstructions(src, typeName string) []goSite {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		return nil
	}
	pkg, name := "", typeName
	if i := strings.LastIndex(typeName, "."); i >= 0 {
		pkg, name = typeName[:i], typeName[i+1:]
	}
	aliased := map[string]string{} // local name -> import path, for aliased imports
	for _, imp := range f.Imports {
		if imp.Name != nil {
			path, _ := strconv.Unquote(imp.Path.Value)
			aliased[imp.Name.Name] = path
		}
	}
	guarded := func(t ast.Expr) bool {
		switch x := t.(type) { // a generic instantiation constructs its base type
		case *ast.IndexExpr:
			t = x.X
		case *ast.IndexListExpr:
			t = x.X
		}
		switch x := t.(type) {
		case *ast.Ident:
			return x.Name == name && (pkg == "" || f.Name.Name == pkg)
		case *ast.SelectorExpr:
			id, ok := x.X.(*ast.Ident)
			if !ok || pkg == "" || x.Sel.Name != name {
				return false
			}
			if path, ok := aliased[id.Name]; ok {
				return importPathName(path) == pkg
			}
			return id.Name == pkg
		}
		return false
	}
	var out []goSite
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch e := n.(type) {
			case *ast.CompositeLit:
				if guarded(e.Type) {
					out = append(out, goSite{typeName, fn.Name.Name, fset.Position(e.Pos()).Line})
				}
			case *ast.CallExpr:
				if id, ok := e.Fun.(*ast.Ident); ok && id.Name == "new" && len(e.Args) == 1 && guarded(e.Args[0]) {
					out = append(out, goSite{typeName, fn.Name.Name, fset.Position(e.Pos()).Line})
				}
			}
			return true
		})
	}
	return out
}

// importPathName is the package name an import path conventionally declares:
// its last element, or the one before a major-version suffix (/v2).
func importPathName(path string) string {
	parts := strings.Split(path, "/")
	last := parts[len(parts)-1]
	if len(parts) > 1 && len(last) > 1 && last[0] == 'v' && strings.Trim(last[1:], "0123456789") == "" {
		last = parts[len(parts)-2]
	}
	return last
}

type goRef struct {
	Name string // the CUE label/definition referenced, e.g. "schemas", "#Lineage", "_#schema"
	Via  string // Str | Def | Hid
	Line int
}

// goStringRefs finds literal references into CUE: cue.Str("x"), cue.Def("#X"),
// cue.Hid("_#x", pkg). Non-literal arguments (variables) cannot be resolved
// statically and are skipped.
func goStringRefs(src string) []goRef {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		return nil
	}
	var out []goRef
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "cue" {
			return true
		}
		switch sel.Sel.Name {
		case "Str", "Def", "Hid":
		default:
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		name, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		out = append(out, goRef{Name: name, Via: sel.Sel.Name, Line: fset.Position(lit.Pos()).Line})
		return true
	})
	return out
}

func typeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return typeName(t.X)
	}
	return ""
}
