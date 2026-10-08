// Read-only AST measurements, not a production dependency.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Site struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	End      int    `json:"end,omitempty"`
	Function string `json:"function,omitempty"`
	Kind     string `json:"kind"`
	Value    string `json:"value"`
	Receiver string `json:"receiver,omitempty"`
	Package  string `json:"package,omitempty"`
}
type Report struct {
	Functions        []Site `json:"functions"`
	Calls            []Site `json:"calls"`
	DBCalls          []Site `json:"db_calls"`
	Literals         []Site `json:"literals"`
	SQL              []Site `json:"sql"`
	UnusedParameters []Site `json:"unused_parameters"`
}

var sqlStart = regexp.MustCompile(`(?is)^\s*(SELECT\b|WITH\b|INSERT\b|UPDATE\b|DELETE\b|CREATE\s+(TABLE|FUNCTION|INDEX)\b|ALTER\s+TABLE\b)`)

func main() {
	root := "server"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	fs := token.NewFileSet()
	report := Report{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, err := parser.ParseFile(fs, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		imports := map[string]string{}
		for _, i := range f.Imports {
			v, _ := strconv.Unquote(i.Path.Value)
			alias := filepath.Base(v)
			if i.Name != nil {
				alias = i.Name.Name
			}
			imports[alias] = v
		}
		spans := []Site{}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			name := fn.Name.Name
			if fn.Recv != nil {
				var b bytes.Buffer
				printer.Fprint(&b, fs, fn.Recv.List[0].Type)
				name = b.String() + "." + name
			}
			site := Site{File: filepath.ToSlash(path), Line: fs.Position(fn.Pos()).Line, End: fs.Position(fn.End()).Line, Kind: "function", Value: name, Package: f.Name.Name}
			spans = append(spans, site)
			report.Functions = append(report.Functions, site)
			if fn.Body != nil && fn.Type.Params != nil {
				for _, field := range fn.Type.Params.List {
					for _, param := range field.Names {
						if param.Name == "_" {
							continue
						}
						uses := 0
						ast.Inspect(fn.Body, func(n ast.Node) bool {
							if id, ok := n.(*ast.Ident); ok && id.Obj == param.Obj && param.Obj != nil {
								uses++
							}
							return true
						})
						if uses == 0 {
							report.UnusedParameters = append(report.UnusedParameters, Site{File: filepath.ToSlash(path), Line: fs.Position(param.Pos()).Line, Function: name, Kind: "unused_parameter", Value: param.Name, Package: f.Name.Name})
						}
					}
				}
			}
		}
		base := func(n ast.Node) Site {
			line := fs.Position(n.Pos()).Line
			s := Site{File: filepath.ToSlash(path), Line: line, End: fs.Position(n.End()).Line, Package: f.Name.Name}
			for _, span := range spans {
				if line >= span.Line && line <= span.End {
					s.Function = span.Value
					break
				}
			}
			return s
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok {
				site := base(lit)
				site.Kind = lit.Kind.String()
				site.Value = lit.Value
				if lit.Kind == token.STRING {
					site.Value, _ = strconv.Unquote(lit.Value)
				}
				report.Literals = append(report.Literals, site)
				if lit.Kind == token.STRING && sqlStart.MatchString(site.Value) {
					site.Kind = "sql"
					report.SQL = append(report.SQL, site)
				}
			}
			if call, ok := n.(*ast.CallExpr); ok {
				site := base(call)
				site.Kind = "call"
				switch fn := call.Fun.(type) {
				case *ast.Ident:
					site.Value = fn.Name
				case *ast.SelectorExpr:
					var b bytes.Buffer
					printer.Fprint(&b, fs, fn.X)
					site.Receiver = b.String()
					site.Value = fn.Sel.Name
					if id, ok := fn.X.(*ast.Ident); ok && imports[id.Name] != "" {
						site.Package = imports[id.Name]
					}
				}
				if site.Value != "" {
					report.Calls = append(report.Calls, site)
				}
				switch site.Value {
				case "Exec", "Query", "QueryRow", "Begin", "BeginTx", "SendBatch", "CopyFrom":
					if strings.HasPrefix(site.File, "server/api/handlers/") && !strings.HasSuffix(site.File, "_test.go") && (site.Receiver == "h.pool" || site.Receiver == "pool" || site.Receiver == "tx" || site.Receiver == "db") {
						report.DBCalls = append(report.DBCalls, site)
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, sites := range [][]Site{report.Functions, report.Calls, report.DBCalls, report.Literals, report.SQL} {
		sort.SliceStable(sites, func(i, j int) bool {
			if sites[i].File == sites[j].File {
				return sites[i].Line < sites[j].Line
			}
			return sites[i].File < sites[j].File
		})
	}
	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(report); err != nil {
		panic(err)
	}
}
