// Run from repo root: go run tools/dispatch_inventory_20261025.go
// Read-only inventory of literal and dynamic NotifyPlayer call sites.
package main

import (
 "encoding/json"
 "go/ast"
 "go/parser"
 "go/token"
 "os"
 "path/filepath"
 "sort"
 "strconv"
 "strings"
)

type row struct { Kind string `json:"kind"`; Source string `json:"source"`; Fields []string `json:"fields"` }
func main(){
 var rows []row
 err:=filepath.WalkDir("server",func(path string,d os.DirEntry,err error)error{
  if err!=nil{return err};if d.IsDir()||!strings.HasSuffix(path,".go")||strings.HasSuffix(path,"_test.go"){return nil}
  set:=token.NewFileSet();file,err:=parser.ParseFile(set,path,nil,parser.SkipObjectResolution);if err!=nil{return err}
  ast.Inspect(file,func(n ast.Node)bool{
   call,ok:=n.(*ast.CallExpr);if !ok{return true};sel,ok:=call.Fun.(*ast.SelectorExpr);if !ok||sel.Sel.Name!="NotifyPlayer"||len(call.Args)<6{return true}
   kind:="<dynamic>";if lit,ok:=call.Args[3].(*ast.BasicLit);ok&&lit.Kind==token.STRING{kind,_=strconv.Unquote(lit.Value)}
   fields:=[]string{};if comp,ok:=call.Args[5].(*ast.CompositeLit);ok{for _,elt:=range comp.Elts{if kv,ok:=elt.(*ast.KeyValueExpr);ok{if lit,ok:=kv.Key.(*ast.BasicLit);ok&&lit.Kind==token.STRING{k,_:=strconv.Unquote(lit.Value);fields=append(fields,k)}}}}
   sort.Strings(fields);rows=append(rows,row{kind,set.Position(call.Pos()).String(),fields});return true
  });return nil
 });if err!=nil{panic(err)}
 sort.Slice(rows,func(i,j int)bool{if rows[i].Kind==rows[j].Kind{return rows[i].Source<rows[j].Source};return rows[i].Kind<rows[j].Kind})
 enc:=json.NewEncoder(os.Stdout);enc.SetIndent("","  ");if err:=enc.Encode(rows);err!=nil{panic(err)}
}
