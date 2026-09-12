// Command server exposes a loopback-only, synthetic Portal UI for browser tests.
package main

import (
	"bytes"
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
	mux.HandleFunc("/api/sessions", func(w http.ResponseWriter, r *http.Request) {
		schedules := []map[string]any{}
		if cookie, err := r.Cookie("portal_browser_schedule"); err == nil && cookie.Value == "1" {
			schedules = append(schedules, map[string]any{
				"id": "deadbeef", "host": "test-host", "session": "test-session",
				"text": "synthetic planned message", "created_at": time.Now().Unix(),
				"first_at": time.Now().Add(time.Hour).Unix(), "repeat": 1, "cancelable": true,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version":       "browser-test",
			"host_count":    1,
			"hosts":         []string{"test-host"},
			"host_versions": map[string]string{"test-host": "browser-test"},
			"sessions": []map[string]any{{
				"host": "test-host", "session": "test-session", "last_activity": time.Now().Unix(),
			}},
			"schedules": schedules,
		})
	})
	mux.HandleFunc("/api/test/enable-schedule", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "portal_browser_schedule", Value: "1", Path: "/", SameSite: http.SameSiteStrictMode})
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		var action struct {
			Action     string `json:"action"`
			Host       string `json:"host"`
			ScheduleID string `json:"schedule_id"`
		}
		if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&action) != nil ||
			action.Action != "cancel_schedule" || action.Host != "test-host" || action.ScheduleID != "deadbeef" {
			http.Error(w, "bad synthetic action", http.StatusBadRequest)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "portal_browser_schedule", Value: "0", Path: "/", SameSite: http.SameSiteStrictMode})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
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
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if bytes.Contains(data, []byte("Portal synthetic paste payload")) {
				if err := conn.WriteMessage(websocket.TextMessage, []byte("\r\nPortal paste round trip received\r\n")); err != nil {
					return
				}
			}
		}
	})

	log.Printf("Portal browser fixture listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
