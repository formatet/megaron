package main

import (
	"formatet/megaron/server/api/handlers"
	"github.com/go-chi/chi/v5"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestAlfatestInfoPublic(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	web, err := handlers.NewWebHandler(nil, nil, filepath.Join(root, "web", "templates"), filepath.Join(root, "web", "static"), nil)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	registerAlfatestInfoRoute(router, web.AlfatestInfo)
	response := httptest.NewRecorder()
	// Intentionally no Authorization header or cookie, and no database service.
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/alfatestinfo", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("public invitation status=%d: %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type=%q", got)
	}
	for _, text := range []string{`<html lang="sv">`, "Det här är en tidig alfa", "Report", "keryx report", `href="/"`, "Bild saknas"} {
		if !strings.Contains(response.Body.String(), text) {
			t.Errorf("rendered invitation is missing %q", text)
		}
	}
}

// The DB-free HTTP test executes the real registrar; this guards its main wiring.
func TestMainUsesPublicAlfatestInfoRegistrar(t *testing.T) {
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
			if name, ok := call.Fun.(*ast.Ident); ok && name.Name == "registerAlfatestInfoRoute" {
				registrations++
				if len(call.Args) != 2 {
					t.Error("registrar needs public router and real handler")
					return true
				}
				if router, ok := call.Args[0].(*ast.Ident); !ok || router.Name != "r" {
					t.Error("invitation must use the public root router")
				}
				if handler, ok := call.Args[1].(*ast.SelectorExpr); !ok || handler.Sel.Name != "AlfatestInfo" {
					t.Error("registrar must serve the real invitation handler")
				}
			}
			if method, ok := call.Fun.(*ast.SelectorExpr); ok && method.Sel.Name == "Get" {
				for _, arg := range call.Args {
					if literal, ok := arg.(*ast.BasicLit); ok && literal.Kind == token.STRING && literal.Value == `"/alfatestinfo"` {
						t.Error("main must use the tested public invitation registrar")
					}
				}
			}
			return true
		})
	}
	if registrations != 1 {
		t.Fatalf("main invitation registrations=%d, want 1", registrations)
	}
}
