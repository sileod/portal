package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sileod/portal/internal/protocol"
)

func TestSessionListPreservesAndClearsCodexReset(t *testing.T) {
	server := New("hash", "token")
	agent := &agentConn{}
	server.agents["host"] = agent
	for _, reset := range []int64{1800000000, 0} {
		applyAgentSnapshot(agent, protocol.Message{SessionInfos: []protocol.Session{{Session: "codex", CodexResetAt: reset}}})
		response := httptest.NewRecorder()
		server.handleSessions(response, httptest.NewRequest(http.MethodGet, "/api/sessions", nil))
		var list protocol.SessionList
		if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		if len(list.Sessions) != 1 || list.Sessions[0].CodexResetAt != reset {
			t.Fatalf("lost reset metadata: %+v", list.Sessions)
		}
	}
}
