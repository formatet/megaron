package handlers

import (
	"net/http"
	"testing"
)

// This test also compiles unchanged on the old implementation: its failure is
// the observable missing identity, rather than a reference to a new Go symbol.
func TestSendCreatesPersistentCaseIdentity(t *testing.T) {
	f := setupReplyFixture(t)
	code, resp := f.post(t, f.senderToken, "/worlds/"+f.worldID.String()+"/settlements/"+f.originID.String()+"/messengers", map[string]any{"destination_id": f.destID.String(), "message": "A first matter", "subject": "Tin for winter"})
	if code != http.StatusCreated {
		t.Fatalf("send: %d %v", code, resp)
	}
	if id, ok := resp["case_id"].(string); !ok || id == "" {
		t.Fatalf("sent letter has no persistent case_id: %v", resp)
	}
}
