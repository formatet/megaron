package handlers

import (
	"context"
	"encoding/json"
	"flag"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/unit"
)

var updateCodexReference = flag.Bool("update-codex-reference", false, "regenerate the Codex facts from authoritative catalogues")

const codexRoot = "../../../web/static/codex"

type codexEntity struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
}
type codexCatalogArticle struct {
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	Aliases  []string      `json:"aliases"`
	Future   bool          `json:"future"`
	Entities []codexEntity `json:"entities"`
	Related  []string      `json:"related"`
}

func typedStringConstants(t *testing.T, path, kind string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Type-check the catalogue's scalar type and constant declarations. This
	// evaluates implicit declarations, aliases and constant expressions without
	// importing runtime/database dependencies from the rest of the source file.
	declarations := []ast.Decl{}
	for _, declaration := range f.Decls {
		g, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		if g.Tok == token.CONST {
			declarations = append(declarations, g)
			continue
		}
		if g.Tok != token.TYPE {
			continue
		}
		for _, spec := range g.Specs {
			t := spec.(*ast.TypeSpec)
			if _, scalar := t.Type.(*ast.Ident); scalar {
				declarations = append(declarations, &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{t}})
			}
		}
	}
	catalogue := &ast.File{Name: f.Name, Decls: declarations}
	pkg, err := new(types.Config).Check("catalogue", fset, []*ast.File{catalogue}, nil)
	if err != nil {
		t.Fatalf("cannot evaluate %s catalogue constants: %v", kind, err)
	}
	var ids []string
	for _, name := range pkg.Scope().Names() {
		c, ok := pkg.Scope().Lookup(name).(*types.Const)
		if !ok {
			continue
		}
		named, ok := c.Type().(*types.Named)
		if !ok || named.Obj().Name() != kind {
			continue
		}
		if c.Val().Kind() != constant.String {
			t.Fatalf("%s.%s is not a string constant", kind, name)
		}
		ids = append(ids, constant.StringVal(c.Val()))
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		t.Fatalf("no %s constants from %s", kind, path)
	}
	return ids
}

// Canonical labels are owned by the same functions used in game projections.
func codexEntityNames(t *testing.T) map[string]map[string]string {
	t.Helper()
	names := map[string]map[string]string{"unit": {}, "building": {}}
	for _, id := range typedStringConstants(t, "../../internal/unit/model.go", "Type") {
		names["unit"][id] = unit.DisplayName(id)
	}
	for id := range province.BuildingSpecs {
		names["building"][string(id)] = province.BuildingDisplayName(string(id))
	}
	return names
}

func TestCodexTypedConstantsIncludeImplicitAndExpressions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model.go")
	source := `package fixture
 type Terrain string
 const (
  Plain Terrain = "plain"
  Same
  Lake = Terrain("lake")
  Deep = Lake + "_deep"
 )`
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	got := typedStringConstants(t, path, "Terrain")
	if !reflect.DeepEqual(got, []string{"lake", "lake_deep", "plain", "plain"}) {
		t.Fatalf("implicit/expression catalogue lost constants: %v", got)
	}
}

func TestCodexEntityCoverageFromRuntimeCatalogues(t *testing.T) {
	pool := navalPlayerTestPool(t)
	defer pool.Close()
	required := map[codexEntity]bool{}
	add := func(kind, key string) { required[codexEntity{kind, key}] = true }
	for _, id := range typedStringConstants(t, "../../internal/unit/model.go", "Type") {
		add("unit", id)
	}
	for id := range province.UnitSpecs {
		add("unit", id)
	}
	for id := range province.BuildingSpecs {
		add("building", string(id))
	}
	for _, id := range typedStringConstants(t, "../../internal/world/model.go", "Terrain") {
		add("terrain", id)
	}
	for _, god := range religion.TempleGods {
		add("god", god.ID)
	}
	for _, id := range religion.CulturePrayers[string(province.MVPCulture)] {
		add("prayer", id)
	}
	// cult is devotion's FK anchor; shipbuilding is implemented service work.
	rows, err := pool.Query(context.Background(), `SELECT key FROM goods WHERE status='active' OR key='shipbuilding' ORDER BY key`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		add("good", key)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(codexRoot, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var idx struct {
		Articles []codexCatalogArticle `json:"articles"`
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatal(err)
	}
	names := codexEntityNames(t)
	seen := map[codexEntity]string{}
	for _, article := range idx.Articles {
		for _, entity := range article.Entities {
			if article.Future {
				t.Errorf("future %s cannot satisfy current entity %+v", article.ID, entity)
				continue
			}
			if !required[entity] {
				t.Errorf("%s claims non-current entity %+v", article.ID, entity)
			}
			if prev := seen[entity]; prev != "" {
				t.Errorf("entity %+v duplicated by %s and %s", entity, prev, article.ID)
			}
			if name := names[entity.Kind][entity.Key]; name != "" {
				found := false
				for _, label := range append([]string{article.Title}, article.Aliases...) {
					if strings.EqualFold(strings.TrimSpace(label), name) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s title/aliases omit canonical %s name %q", article.ID, entity.Kind, name)
				}
			}
			seen[entity] = article.ID
		}
	}
	var missing []string
	for entity := range required {
		if seen[entity] == "" {
			missing = append(missing, entity.Kind+":"+entity.Key)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("current catalogue entities without article: %v", missing)
	}
}

func TestCodexReferenceMatchesAuthoritativeCatalogues(t *testing.T) {
	pool := navalPlayerTestPool(t)
	defer pool.Close()
	h := NewProvinceHandler(pool, nil, nil, nil, nil)
	decode := func(handler func(*httptest.ResponseRecorder)) any {
		r := httptest.NewRecorder()
		handler(r)
		if r.Code != 200 {
			t.Fatalf("catalogue %d: %s", r.Code, r.Body.String())
		}
		var data any
		if err := json.Unmarshal(r.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		return data
	}
	facts := map[string]any{
		"units": decode(func(w *httptest.ResponseRecorder) {
			h.UnitCatalogue(w, httptest.NewRequest("GET", "/api/v1/units", nil))
		}),
		"buildings": decode(func(w *httptest.ResponseRecorder) {
			h.BuildingCatalogue(w, httptest.NewRequest("GET", "/api/v1/buildings", nil))
		}),
		"recipes": decode(func(w *httptest.ResponseRecorder) {
			h.RecipeCatalogue(w, httptest.NewRequest("GET", "/api/v1/recipes", nil))
		}),
		"gods":         religion.TempleGods,
		"entity_names": codexEntityNames(t),
	}
	prayers := map[string]religion.PrayerSpec{}
	for _, id := range religion.CulturePrayers[string(province.MVPCulture)] {
		prayers[id] = religion.PrayerSpecs[id]
	}
	facts["prayers"] = prayers
	var goodsRaw []byte
	if err := pool.QueryRow(context.Background(), `SELECT json_agg(t) FROM(SELECT key,name,tier,category,status,weight FROM goods ORDER BY key)t`).Scan(&goodsRaw); err != nil {
		t.Fatal(err)
	}
	var goods any
	if err := json.Unmarshal(goodsRaw, &goods); err != nil {
		t.Fatal(err)
	}
	facts["goods"] = goods
	facts["terrain_ids"] = typedStringConstants(t, "../../internal/world/model.go", "Terrain")
	data, err := json.MarshalIndent(facts, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	path := filepath.Join(codexRoot, "reference.json")
	if *updateCodexReference {
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var actualJSON, wantJSON any
	if err := json.Unmarshal(data, &actualJSON); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &wantJSON); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actualJSON, wantJSON) {
		t.Fatal("Codex reference drifted: regenerate from authoritative catalogues with -update-codex-reference")
	}
}

// Numeric rule guards follow the overview’s actual See also links after a split.
func codexArticleTextWithRelated(t *testing.T, id string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(codexRoot, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var idx struct {
		Articles []struct {
			ID       string   `json:"id"`
			Related  []string `json:"related"`
			Sections []struct {
				ID string `json:"id"`
			} `json:"sections"`
		} `json:"articles"`
	}
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatal(err)
	}
	// A related entry may name a section; its text lives in the article's file.
	file := map[string]string{}
	for _, a := range idx.Articles {
		file[a.ID] = a.ID
		for _, s := range a.Sections {
			file[s.ID] = a.ID
		}
	}
	ids := []string{id}
	for _, a := range idx.Articles {
		if a.ID == id {
			for _, r := range a.Related {
				ids = append(ids, file[r])
			}
		}
	}
	text := ""
	for _, id := range ids {
		raw, err := os.ReadFile(filepath.Join(codexRoot, id+".md"))
		if err != nil {
			t.Fatal(err)
		}
		text += "\n" + string(raw)
	}
	return text
}
