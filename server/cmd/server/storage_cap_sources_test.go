package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// These INSERT-cap consumers must use the shared owner, not a private literal.
// Database tests separately cover the resulting cap and conflict behaviour.
func TestDefaultGoodStorageCapConsumers(t *testing.T) {
	for _, name := range []string{"api/handlers/logistics.go", "api/handlers/create_metropolis.go", "internal/combat/unit_arrival.go", "internal/transport/arrival.go", "internal/transport/intercept.go", "internal/economy/trade.go", "internal/economy/recompute.go"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile("../../" + name)
			if err != nil {
				t.Fatal(err)
			}
			file, err := parser.ParseFile(token.NewFileSet(), name, data, 0)
			if err != nil {
				t.Fatal(err)
			}
			shared := false
			ast.Inspect(file, func(n ast.Node) bool {
				if s, ok := n.(*ast.SelectorExpr); ok && s.Sel.Name == "DefaultGoodStorageCap" {
					shared = true
				}
				if lit, ok := n.(*ast.BasicLit); ok && strings.Contains(strings.ReplaceAll(lit.Value, "_", ""), "1000000") {
					t.Errorf("private storage ceiling in %s", lit.Value)
				}
				return true
			})
			if !shared {
				t.Error("consumer does not read DefaultGoodStorageCap")
			}
		})
	}
}
