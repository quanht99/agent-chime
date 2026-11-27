package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

// parse reads every .go file in the package, ignoring build tags.
func parse(t *testing.T) map[string]*ast.File {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*ast.File{}
	for _, p := range paths {
		if filepath.Base(p) == "source_test.go" {
			continue
		}
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(token.NewFileSet(), p, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		out[p] = f
	}
	return out
}

// selector returns "a.b.c" for a selector chain, or "".
func selector(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		if p := selector(x.X); p != "" {
			return p + "." + x.Sel.Name
		}
	}
	return ""
}

func refs(f *ast.File) (sels map[string]bool, idents map[string]bool) {
	sels, idents = map[string]bool{}, map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			if s := selector(x); s != "" {
				sels[s] = true
			}
			idents[x.Sel.Name] = true
		case *ast.Ident:
			idents[x.Name] = true
		}
		return true
	})
	return
}

func TestNoArguments(t *testing.T) {
	files := parse(t)
	if len(files) == 0 {
		t.Fatal("no source files")
	}
	for name, f := range files {
		sels, _ := refs(f)
		if sels["os.Args"] {
			t.Errorf("%s references os.Args", name)
		}
		for _, imp := range f.Imports {
			if imp.Path.Value == `"flag"` {
				t.Errorf("%s imports flag", name)
			}
		}
	}
}

func TestReadyWaitsForThePage(t *testing.T) {
	files := parse(t)
	want := map[string]string{
		"ready_windows.go": "events.Windows.WebViewNavigationCompleted",
		"ready_linux.go":   "events.Linux.WindowLoadFinished",
	}
	for name, sel := range want {
		f, ok := files[name]
		if !ok {
			t.Errorf("%s is missing", name)
			continue
		}
		if sels, _ := refs(f); !sels[sel] {
			t.Errorf("%s does not reference %s", name, sel)
		}
	}
	for name, f := range files {
		if sels, _ := refs(f); sels["events.Common.WindowShow"] {
			t.Errorf("%s references events.Common.WindowShow", name)
		}
	}
}

func TestNoBindings(t *testing.T) {
	f, ok := parse(t)["main_gui.go"]
	if !ok {
		t.Fatal("main_gui.go is missing")
	}
	_, idents := refs(f)
	for _, id := range []string{"Services", "Assets", "Bind", "NewService"} {
		if idents[id] {
			t.Errorf("main_gui.go uses %s", id)
		}
	}
}
