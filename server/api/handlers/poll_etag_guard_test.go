package handlers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// The web client polls these every 3 or 30 seconds with If-None-Match
// (api.js pollJSON). A handler that drops writeJSONETag still works, but
// silently sends and reparses the full body on every poll again.
func TestPollHandlersAnswerWithETag(t *testing.T) {
	polled := map[string]bool{
		"WorldHandler.Get": false, "WorldHandler.Map": false, "WorldHandler.Storms": false,
		"WorldHandler.Provinces": false, "WorldHandler.Marches": false, "WorldHandler.MapMessengers": false,
		"WorldHandler.MapTrades": false, "WorldHandler.ForeignUnits": false, "WorldHandler.RuralProjections": false,
		"UnitHandler.ListUnits": false, "UnitHandler.Roads": false, "UnitHandler.RoadSites": false,
		"SettlementHandler.PlacementRoster": false,
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			recv, ok := star.X.(*ast.Ident)
			if !ok {
				continue
			}
			key := recv.Name + "." + fn.Name.Name
			if _, ok := polled[key]; !ok {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "writeJSONETag" {
						polled[key] = true
					}
				}
				return true
			})
		}
	}
	for key, ok := range polled {
		if !ok {
			t.Errorf("%s does not answer with writeJSONETag", key)
		}
	}
}
