package messenger

import (
	"strings"
	"testing"
)

func TestStalledExplanation_SaysWhatWhyAndWhatToDo(t *testing.T) {
	kydonia := "Kydonia"
	foreign := stalledExplanation("Amnissos", &kydonia, false, 0)
	for _, want := range []string{"Amnissos", "Kydonia", "another Wanax's", "by itself", "send one from a port of yours"} {
		if !strings.Contains(foreign, want) {
			t.Errorf("foreign-port explanation %q lacks %q", foreign, want)
		}
	}
	if strings.Contains(foreign, "call it back") {
		t.Errorf("a runner in a foreign port cannot be called back, but the text offers it: %q", foreign)
	}
	own := stalledExplanation("Phalasarna", nil, true, 2)
	for _, want := range []string{"Phalasarna", "its destination", "send one of your own ships now", "call it back"} {
		if !strings.Contains(own, want) {
			t.Errorf("own-port explanation %q lacks %q", own, want)
		}
	}
}
