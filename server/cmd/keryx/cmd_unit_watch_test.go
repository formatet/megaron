package main

import (
	"strings"
	"testing"
)

// `unit watch` och `unit sentry` är TVÅ ORDER, inte två namn på samma sak, och
// den enda skillnaden i anropet är ett fältnamn:
//
//	sentry (sjö):  {"intent": "sentry"}  → seglar, patrullerar, vänder HEM själv
//	watch  (land): {"stance": "sentry"}  → marscherar och STÅR KVAR
//
// Blandar man ihop dem får spelaren tillbaka precis det beteende hela raden
// existerar för att komma ifrån: en enhet som åker ut och kommer hem igen.
// Testet spikar därför fältnamnet, inte bara att kommandot finns.
func TestUnitWatch_SendsStanceNotIntent(t *testing.T) {
	cmd := unitWatchCmd()

	if got := cmd.Name(); got != "watch" {
		t.Fatalf("kommandot heter %q, vill ha \"watch\"", got)
	}

	// Marschvägen är delad med `march` — watch får inte uppfinna en egen
	// endpoint, bara en egen förvald hållning.
	for _, flag := range []string{"unit", "q", "r", "target"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("flaggan --%s saknas", flag)
		}
	}

	// Hjälptexten måste bära bådadera: att den STÅR KVAR (skillnaden mot
	// sentry och mot explore) och att den kostar dubbelt (annars upptäcks
	// priset genom svält).
	long := strings.ToLower(cmd.Long)
	if !strings.Contains(long, "stays") && !strings.Contains(long, "no auto-return") {
		t.Error("Long säger inte att enheten står kvar — det är hela skillnaden mot 'unit sentry'")
	}
	if !strings.Contains(long, "double") {
		t.Error("Long nämner inte att fältransonen är dubbel; priset ska stå i hjälpen, inte upptäckas genom svält")
	}
	if !strings.Contains(long, "explore") {
		t.Error("Long ställer inte watch mot explore — det är den förväxling raden finns för att lösa")
	}
}

// Målformen ska vara samma som march/redirect, inte en tredje egen. Rad I i
// cli_sanning städade bort exakt den sortens drift; det här hindrar återfall.
func TestUnitWatch_UsesTheSharedTargetForm(t *testing.T) {
	cmd := unitWatchCmd()
	if err := cmd.ParseFlags([]string{"--unit", "abc", "--target", "8,-3"}); err != nil {
		t.Fatalf("parse --target: %v", err)
	}
	q, r, given, err := resolveTargetHex(cmd, "8,-3", 0, 0)
	if err != nil {
		t.Fatalf("resolveTargetHex: %v", err)
	}
	if !given || q != 8 || r != -3 {
		t.Errorf("resolveTargetHex gav (%d,%d,given=%v), vill ha (8,-3,true)", q, r, given)
	}
}
