package capabilities

import (
	"reflect"
	"strings"
	"testing"
)

// Discovery-legibiliteten (megaron_todo.md). Roten var inte att motorn saknade
// den framskjutna posten — den har funnits sedan sentry byggdes — utan att INGEN
// YTA pekade på den. Varje mening i actions som rörde okänd mark namngav
// `explore`, den enda order som vänder hem igen.
//
// Testet vaktar de tre meningarna som bär den fixen. De är text, och text är
// precis det som tyst glider tillbaka.
func TestMarchPurpose_NamesBothWaysOut(t *testing.T) {
	p := purposeOf(t, "march")

	if !strings.Contains(p, "explore") {
		t.Error("march nämner inte explore alls")
	}
	// Det avgörande: explore måste beskrivas som en RUNDRESA, annars läser den
	// fortfarande som vägen ut i det okända utan hake.
	if !strings.Contains(strings.ToLower(p), "turns for home") {
		t.Error("march säger inte att explore vänder hem — utan det står rundresan kvar " +
			"som den enda synliga vägen mot okänd mark")
	}
	if !strings.Contains(strings.ToLower(p), "sentry") {
		t.Error("march pekar inte på sentry/watch som alternativet som stannar")
	}
}

func TestStancePurpose_SaysWhatSentryDoes(t *testing.T) {
	p := strings.ToLower(purposeOf(t, "stance"))
	for _, want := range []string{"sentry", "intercept"} {
		if !strings.Contains(p, want) {
			t.Errorf("stance beskriver inte %q — den listade bara hållningarnas namn", want)
		}
	}
	// Den vanligaste felslutsatsen: att man behöver sentry för att SE.
	// LoadLiveEyes läser aldrig stance; varje enhet på kartan är redan ett öga.
	if !strings.Contains(p, "does not") && !strings.Contains(p, "not need") {
		t.Error("stance säger inte att sentry INTE krävs för att se — den missförståndet " +
			"är vad som får spelare att tro att en vanlig marsch är blind")
	}
}

// Ett skepp postas med `unit patrol` (patrulltimer, seglar hem själv), en
// landenhet med `watch` (står kvar). Kravet måste därför vara LAND — annars
// erbjuds watch till en spelare vars enda enhet är en galär, och hen får en
// helt annan order än ytan lovade.
func TestPostVerb_IsGatedOnLandNotJustAnyUnit(t *testing.T) {
	// Att verbet finns i registret är en egen sak från vad det säger — utan
	// registrering syns det aldrig i `keryx actions`.
	var registered bool
	for _, check := range checkers {
		if fnName(check) == fnName(canPost) {
			registered = true
			break
		}
	}
	if !registered {
		t.Fatal("post finns inte i checkers-registret — då syns den aldrig i `keryx actions`")
	}

	{
		v := canPost(checkContext{})
		if v.Category != CategoryMilitary {
			t.Errorf("post ligger i kategori %q, vill ha %q", v.Category, CategoryMilitary)
		}
		if len(v.Requirements) == 0 {
			t.Fatal("post bär inga krav alls")
		}
		req := v.Requirements[0]
		if !strings.Contains(strings.ToLower(req.Text), "land") {
			t.Errorf("postens krav lyder %q — det måste säga LAND, ett skepp postas med `unit patrol`", req.Text)
		}
		if !strings.Contains(strings.ToLower(req.Hint), "patrol") {
			t.Error("postens hint pekar inte den som bara har skepp mot `unit patrol`")
		}
		if !strings.Contains(strings.ToLower(v.Purpose), "double") {
			t.Error("postens purpose nämner inte den dubbla fältransonen — en stående " +
				"utgift ska stå i erbjudandet, inte upptäckas efteråt")
		}
	}
}

// fnName identifierar en checker på funktionspekare, eftersom Verb.Name inte går
// att läsa ur alla checkers utan en pool.
func fnName(f func(checkContext) Verb) uintptr {
	return reflect.ValueOf(f).Pointer()
}

// purposeOf plockar purpose-strängen ur EN namngiven checker. Den loopar
// medvetet inte över hela `checkers`: flera av dem (canAbandon m.fl.) läser
// databasen ovillkorligt och panikar mot ett tomt checkContext. De tre verb som
// testas här är nil-pool-säkra, så de anropas direkt.
func purposeOf(t *testing.T, name string) string {
	t.Helper()
	byName := map[string]func(checkContext) Verb{
		"march":  canMarch,
		"stance": canStance,
		"post":   canPost,
	}
	check, ok := byName[name]
	if !ok {
		t.Fatalf("purposeOf känner inte till %q", name)
	}
	return check(checkContext{}).Purpose
}
