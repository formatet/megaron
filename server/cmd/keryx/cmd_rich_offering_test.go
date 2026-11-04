package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRichOfferingCmdPutsAndDeletes(t *testing.T) {
	var calls []string
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/cult") {
			fmt.Fprint(w, `{"kharis":{},"temples":[{"id":"t1","province_id":"p1","god_id":"potnia"},{"id":"t2","province_id":"p2","god_id":"asterion"}],"prayers":[],"unbuilt_temples":[]}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		calls = append(calls, r.Method+" "+r.URL.Path+" "+string(b))
		fmt.Fprint(w, `{}`)
	})
	run := func(args ...string) error {
		c := richOfferingCmd()
		c.SetArgs(args)
		_, err := captureStdout(t, c.Execute)
		return err
	}
	if err := run("--god", "potnia", "--good", "fish"); err != nil {
		t.Fatal(err)
	}
	if err := run("--god", "potnia", "--off"); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || !strings.HasPrefix(calls[0], "PUT ") || !strings.Contains(calls[0], "/temples/t1/rich-offering") || !strings.Contains(calls[0], `"good":"fish"`) || !strings.HasPrefix(calls[1], "DELETE ") {
		t.Fatalf("calls %v", calls)
	}
	if run("--god", "potnia") == nil || run("--god", "nobody", "--off") == nil {
		t.Fatal("missing choice or temple must fail")
	}
}

func TestTempleOfferingLineShowsRichOffering(t *testing.T) {
	row := map[string]any{"name": "Knossos", "temple": "Temple of Potnia", "rich_offering_good": "fish", "rich_offering_paid": true}
	if got := templeOfferingLine(row); !strings.Contains(got, "rich offering fish paid") {
		t.Fatal(got)
	}
	row["rich_offering_paid"] = false
	if got := templeOfferingLine(row); !strings.Contains(got, "rich offering fish not paid") {
		t.Fatal(got)
	}
}
