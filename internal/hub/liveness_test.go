package hub

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sileod/portal/internal/protocol"
)

func testHub(t *testing.T, configure func(*Server)) (*Server, string) {
	t.Helper()
	s := New("unused", "agent-token")
	configure(s)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent", s.handleAgent)
	mux.HandleFunc("/api/terminal", s.handleTerminal)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return s, "ws" + strings.TrimPrefix(srv.URL, "http")
}

func dialAgent(t *testing.T, s *Server, base string) *websocket.Conn {
	t.Helper()
	header := http.Header{"Authorization": {"Bearer agent-token"}}
	ws, _, err := websocket.DefaultDialer.Dial(base+"/api/agent", header)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	if err := ws.WriteJSON(protocol.Message{Type: "hello", Host: "h", Sessions: []string{"a", "b"}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { s.mu.RLock(); defer s.mu.RUnlock(); return s.agents["h"] != nil })
	return ws
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStalledBrowserDoesNotBlockOtherTerminals(t *testing.T) {
	s, base := testHub(t, func(s *Server) { s.browserQueue = 8 })
	agent := dialAgent(t, s, base)

	stalled, _, err := websocket.DefaultDialer.Dial(base+"/api/terminal?host=h&session=a", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stalled.Close() // never read: its socket buffers fill up
	live, _, err := websocket.DefaultDialer.Dial(base+"/api/terminal?host=h&session=b", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()

	ids := map[string]string{}
	for len(ids) < 2 {
		var m protocol.Message
		if err := agent.ReadJSON(&m); err != nil {
			t.Fatal(err)
		}
		if m.Type == "open" {
			ids[m.Session] = m.ID
		}
	}

	chunk := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("x"), 32<<10))
	go func() {
		for i := 0; i < 1024; i++ {
			if agent.WriteJSON(protocol.Message{Type: "output", ID: ids["a"], Data: chunk}) != nil {
				return
			}
		}
		_ = agent.WriteJSON(protocol.Message{Type: "output", ID: ids["b"], Data: base64.StdEncoding.EncodeToString([]byte("still flowing"))})
	}()

	_ = live.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, data, err := live.ReadMessage()
	if err != nil {
		t.Fatalf("live terminal starved behind a stalled one: %v", err)
	}
	if string(data) != "still flowing" {
		t.Fatalf("live terminal got %q", data)
	}
}

func TestSilentHostIsDropped(t *testing.T) {
	s, base := testHub(t, func(s *Server) {
		s.pingPeriod, s.pongWait = 50*time.Millisecond, 200*time.Millisecond
	})
	dialAgent(t, s, base) // never reads, so it never answers pings, like a half-open link

	waitFor(t, func() bool { s.mu.RLock(); defer s.mu.RUnlock(); return s.agents["h"] == nil })
}

func TestSecondMachineWithSameLabelDoesNotEvictFirst(t *testing.T) {
	s, base := testHub(t, func(*Server) {})
	connect := func(machine string) *websocket.Conn {
		header := http.Header{"Authorization": {"Bearer agent-token"}}
		ws, _, err := websocket.DefaultDialer.Dial(base+"/api/agent", header)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ws.Close() })
		if err := ws.WriteJSON(protocol.Message{Type: "hello", Host: "portal", Machine: machine}); err != nil {
			t.Fatal(err)
		}
		return ws
	}
	agentsLen := func() int { s.mu.RLock(); defer s.mu.RUnlock(); return len(s.agents) }

	connect("magnet10")
	waitFor(t, func() bool { return agentsLen() == 1 })
	second := connect("magnet.6")
	waitFor(t, func() bool { return agentsLen() == 2 })
	var m protocol.Message
	if err := second.ReadJSON(&m); err != nil || m.Type != "host" || m.Host != "magnet-6" {
		t.Fatalf("second machine told %+v, %v; want host magnet-6", m, err)
	}
	s.mu.RLock()
	first, renamed := s.agents["portal"], s.agents["magnet-6"]
	s.mu.RUnlock()
	if first == nil || first.machine != "magnet10" || renamed == nil || renamed.machine != "magnet.6" {
		t.Fatalf("agents = portal:%v magnet-6:%v", first, renamed)
	}

	// A reconnect from the same machine still replaces its old connection.
	connect("magnet10")
	waitFor(t, func() bool { s.mu.RLock(); defer s.mu.RUnlock(); return s.agents["portal"] != first })
	if n := agentsLen(); n != 2 {
		t.Fatalf("agents = %d after reconnect, want 2", n)
	}
}
