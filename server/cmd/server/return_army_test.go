package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// This executes the production registrar, without starting main's database,
// workers or auth stack. The source guard below checks main's wiring separately.
func TestReturnArmyKingdomsGate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled string
		status  int
		called  bool
	}{
		{"default disabled", "", http.StatusForbidden, false},
		{"enabled delegates", "1", http.StatusNoContent, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("KINGDOMS_ENABLED", tc.enabled)
			called := false
			r := chi.NewRouter()
			registerReturnArmyRoute(r, func(w http.ResponseWriter, req *http.Request) {
				called = true
				if chi.URLParam(req, "worldID") != "world" || chi.URLParam(req, "settlementID") != "city" {
					t.Error("production route lost world or settlement parameter")
				}
				w.WriteHeader(http.StatusNoContent)
			})
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/worlds/world/settlements/city/return-army", nil))
			if rec.Code != tc.status || called != tc.called {
				t.Fatalf("status=%d, handler called=%v; want %d, %v", rec.Code, called, tc.status, tc.called)
			}
			if !tc.called {
				var body map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] != "kingdoms_disabled" {
					t.Fatalf("disabled response = %s (%v)", rec.Body.String(), err)
				}
			}
		})
	}
}

// main builds its router inline. Verify it uses the tested registrar with the
// real handler, and cannot silently reintroduce a direct unguarded registration.
func TestMainUsesReturnArmyRegistrar(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	registrations := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "main" {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if name, ok := call.Fun.(*ast.Ident); ok && name.Name == "registerReturnArmyRoute" {
				registrations++
				if len(call.Args) != 2 {
					t.Error("ReturnArmy registrar must receive router and handler")
				} else if handler, ok := call.Args[1].(*ast.SelectorExpr); !ok || handler.Sel.Name != "ReturnArmy" {
					t.Error("production registration must pass ReturnArmy")
				}
			}
			if method, ok := call.Fun.(*ast.SelectorExpr); ok && method.Sel.Name == "Post" {
				for _, arg := range call.Args {
					if literal, ok := arg.(*ast.BasicLit); ok && literal.Kind == token.STRING && strings.Contains(literal.Value, "/return-army") {
						t.Error("main must register ReturnArmy through the gated production registrar")
					}
				}
			}
			return true
		})
	}
	if registrations != 1 {
		t.Errorf("main calls ReturnArmy registrar %d times, want 1", registrations)
	}
}
