package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The KNOWN-set that FOW-gates letters, trade offers, gifts and transfers has one
// owner: province.VisibleOrigins (megaron_agarkarta v1, slice F). A private copy
// drifted silently once already (capabilities.visibleOrigins, removed 2026-10-08);
// two copies that disagree let a player reach a city they never contacted.
func TestVisibleOriginsHasOneOwner(t *testing.T) {
	const owner = "internal/province/contacts.go"
	root := "../.."
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if rel == owner {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), rel, data, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.FuncDecl:
				if strings.EqualFold(v.Name.Name, "visibleOrigins") {
					t.Errorf("%s: private contact-set function %s — call province.VisibleOrigins", rel, v.Name.Name)
				}
			case *ast.BasicLit:
				if v.Kind == token.STRING && strings.Contains(v.Value, "player_scouted_tiles") &&
					strings.Contains(v.Value, "marching_armies") && strings.Contains(v.Value, "messengers") {
					t.Errorf("%s: contact-set SQL copied outside %s", rel, owner)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
