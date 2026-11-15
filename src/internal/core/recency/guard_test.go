package recency

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The one rule for how old a key event is lives here. A key event's time is
// read through inventory.KeyEvents.Time (the only method named Time in the
// tree) or its At map, so a call to .Time(x) outside this package, or an
// index into .At outside core/inventory, is a second place ageing keys, and
// the CLI and the web would drift apart. Positive control: this package's own
// read is found. Floor: the walk must have parsed the tree.
func TestOnlyRecencyAgesAKeyEvent(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	files, ownReads := 0, 0
	err := filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		files++
		rel := filepath.ToSlash(path)
		inRecency := strings.Contains(rel, "core/recency/")
		inInventory := strings.Contains(rel, "core/inventory/")
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Time" && len(x.Args) == 1 {
					if inRecency {
						ownReads++
					} else {
						t.Errorf("%s reads a key event's time (%s); ask core/recency instead", rel, fset.Position(x.Pos()))
					}
				}
			case *ast.IndexExpr:
				if sel, ok := x.X.(*ast.SelectorExpr); ok && sel.Sel.Name == "At" && !inInventory {
					t.Errorf("%s indexes a key's event times (%s); ask core/recency instead", rel, fset.Position(x.Pos()))
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 300 {
		t.Fatalf("parsed %d files; the walk is not reaching the tree", files)
	}
	if ownReads == 0 {
		t.Fatal("found no read in core/recency itself; the matcher cannot see one")
	}
}
