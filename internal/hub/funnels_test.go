package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sileod/portal/internal/protocol"
)

func TestSessionListIncludesFunnelsOnlyForConnectedHosts(t *testing.T) {
	s := New("hash", "token")
	s.agents["apps"] = &agentConn{funnels: []protocol.Funnel{{URL: "https://apps.example/opencode"}}}
	snapshot := func() protocol.SessionList {
		w := httptest.NewRecorder()
		s.handleSessions(w, httptest.NewRequest(http.MethodGet, "/api/sessions", nil))
		var list protocol.SessionList
		if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		return list
	}
	if list := snapshot(); len(list.HostFunnels["apps"]) != 1 || list.HostFunnels["apps"][0].URL != "https://apps.example/opencode" {
		t.Fatalf("missing funnels: %+v", list.HostFunnels)
	}
	delete(s.agents, "apps")
	if list := snapshot(); len(list.HostFunnels) != 0 {
		t.Fatalf("stale funnels: %+v", list.HostFunnels)
	}
}
