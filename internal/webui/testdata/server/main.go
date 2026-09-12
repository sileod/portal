// Command server exposes a loopback-only, synthetic Portal UI for browser tests.
package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sileod/portal/internal/webui"
)

func main() {
	addr := os.Getenv("PORTAL_BROWSER_TEST_ADDR")
	if addr == "" {
		addr = "127.0.0.1:18082"
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil || (host != "localhost" && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback())) {
		log.Fatal("browser test server must listen on loopback")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(webui.IndexHTML)
	})
	mux.HandleFunc("/api/sessions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version":       "browser-test",
			"host_count":    1,
			"hosts":         []string{"test-host"},
			"host_versions": map[string]string{"test-host": "browser-test"},
			"sessions": []map[string]any{{
				"host": "test-host", "session": "test-session", "last_activity": time.Now().Unix(),
			}},
			"schedules": []any{},
		})
	})
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
		return r.Host == addr
	}}
	mux.HandleFunc("/api/terminal", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Match tmux mouse mode so plain-drag selection cannot pass accidentally in
		// the terminal's easier, mouse-reporting-disabled state.
		if err := conn.WriteMessage(websocket.TextMessage, []byte("\x1b[?1000h\x1b[?1002h\x1b[?1006hPortal clipboard fixture text\r\nsecond synthetic line\r\n")); err != nil {
			return
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})

	log.Printf("Portal browser fixture listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
