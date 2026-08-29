package hub

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sileod/portal/internal/protocol"
	"github.com/sileod/portal/internal/webui"
)

type agentConn struct {
	host     string
	ws       *websocket.Conn
	writeMu  sync.Mutex
	sessions []string
}

type browserConn struct {
	host    string
	ws      *websocket.Conn
	writeMu sync.Mutex
}

type Server struct {
	token          string
	browserSession string
	mu             sync.RWMutex
	agents         map[string]*agentConn
	routes         map[string]*browserConn
	upgrader       websocket.Upgrader
}

func New(token string) *Server {
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte("portal-browser-session-v1"))
	return &Server{
		token:          token,
		browserSession: hex.EncodeToString(mac.Sum(nil)),
		agents:         map[string]*agentConn{},
		routes:         map[string]*browserConn{},
		upgrader: websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			u, err := url.Parse(origin)
			return err == nil && strings.EqualFold(u.Host, r.Host)
		}},
	}
}

func (s *Server) Run(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/api/sessions", s.requireBrowser(s.handleSessions))
	mux.HandleFunc("/api/terminal", s.requireBrowser(s.handleTerminal))
	mux.HandleFunc("/api/agent", s.handleAgent)
	server := &http.Server{Addr: addr, Handler: securityHeaders(mux), ReadHeaderTimeout: 10 * time.Second}
	log.Printf("portal hub listening on %s", addr)
	return server.ListenAndServe()
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; connect-src 'self' ws: wss:; font-src 'self' data:")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) browserOK(r *http.Request) bool {
	c, err := r.Cookie("portal_session")
	return err == nil && constantEqual(c.Value, s.browserSession)
}

func (s *Server) requireBrowser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.browserOK(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if !s.browserOK(r) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(webui.IndexHTML)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil || !constantEqual(r.FormValue("token"), s.token) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, loginPage("wrong token"))
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "portal_session", Value: s.browserSession, Path: "/", HttpOnly: true, Secure: isSecure(r), SameSite: http.SameSiteStrictMode, MaxAge: 30 * 24 * 3600})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if s.browserOK(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, loginPage(""))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "portal_session", Value: "", Path: "/", HttpOnly: true, Secure: isSecure(r), SameSite: http.SameSiteStrictMode, MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func loginPage(errText string) string {
	errHTML := ""
	if errText != "" {
		errHTML = `<p class="error">` + errText + `</p>`
	}
	return `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><title>Portal</title><style>:root{color-scheme:light dark}*{box-sizing:border-box}body{margin:0;height:100vh;display:grid;place-items:center;font:14px ui-monospace,SFMono-Regular,Menlo,monospace}form{width:min(360px,calc(100vw - 40px))}h1{font-size:20px}input,button{width:100%;padding:12px;font:inherit;margin-top:8px}button{cursor:pointer}.error{color:#c33}</style></head><body><form method="post"><h1>Portal</h1><input name="token" type="password" autocomplete="current-password" autofocus placeholder="access token"><button>Enter</button>` + errHTML + `</form></body></html>`
}

func isSecure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func constantEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (s *Server) handleSessions(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	list := make([]protocol.Session, 0)
	for host, a := range s.agents {
		for _, session := range a.sessions {
			list = append(list, protocol.Session{Host: host, Session: session})
		}
	}
	hostCount := len(s.agents)
	s.mu.RUnlock()
	sort.Slice(list, func(i, j int) bool {
		if list[i].Host == list[j].Host {
			return list[i].Session < list[j].Session
		}
		return list[i].Host < list[j].Host
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(protocol.SessionList{HostCount: hostCount, Sessions: list})
}

func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	if !constantEqual(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), s.token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	var hello protocol.Message
	if err := ws.ReadJSON(&hello); err != nil || hello.Type != "hello" || hello.Host == "" {
		return
	}
	a := &agentConn{host: hello.Host, ws: ws, sessions: hello.Sessions}
	s.mu.Lock()
	old := s.agents[a.host]
	s.agents[a.host] = a
	s.mu.Unlock()
	if old != nil && old != a {
		old.ws.Close()
	}
	log.Printf("host connected: %s", a.host)
	defer s.removeAgent(a)
	for {
		var m protocol.Message
		if err := ws.ReadJSON(&m); err != nil {
			return
		}
		switch m.Type {
		case "sessions":
			s.mu.Lock()
			if s.agents[a.host] == a {
				a.sessions = append([]string(nil), m.Sessions...)
			}
			s.mu.Unlock()
		case "output":
			data, err := base64.StdEncoding.DecodeString(m.Data)
			if err == nil {
				s.writeBrowser(m.ID, websocket.BinaryMessage, data)
			}
		case "error":
			s.writeBrowser(m.ID, websocket.TextMessage, []byte("\r\n\x1b[31m[portal: "+m.Error+"]\x1b[0m\r\n"))
		}
	}
}

func (s *Server) removeAgent(a *agentConn) {
	s.mu.Lock()
	if s.agents[a.host] == a {
		delete(s.agents, a.host)
	}
	var closeRoutes []*browserConn
	for id, b := range s.routes {
		if b.host == a.host {
			delete(s.routes, id)
			closeRoutes = append(closeRoutes, b)
		}
	}
	s.mu.Unlock()
	for _, b := range closeRoutes {
		b.ws.Close()
	}
	log.Printf("host disconnected: %s", a.host)
}

func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	host, session := r.URL.Query().Get("host"), r.URL.Query().Get("session")
	if host == "" || session == "" {
		http.Error(w, "host and session required", http.StatusBadRequest)
		return
	}
	s.mu.RLock()
	a := s.agents[host]
	s.mu.RUnlock()
	if a == nil {
		http.Error(w, "host offline", http.StatusNotFound)
		return
	}
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	id := randomID()
	b := &browserConn{host: host, ws: ws}
	s.mu.Lock()
	s.routes[id] = b
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.routes, id)
		s.mu.Unlock()
		s.writeAgent(a, protocol.Message{Type: "close", ID: id})
	}()
	if err := s.writeAgent(a, protocol.Message{Type: "open", ID: id, Session: session}); err != nil {
		return
	}
	for {
		messageType, data, err := ws.ReadMessage()
		if err != nil {
			return
		}
		if messageType == websocket.BinaryMessage {
			if err := s.writeAgent(a, protocol.Message{Type: "input", ID: id, Data: base64.StdEncoding.EncodeToString(data)}); err != nil {
				return
			}
			continue
		}
		if messageType == websocket.TextMessage {
			var resize struct {
				Type string `json:"type"`
				Cols uint16 `json:"cols"`
				Rows uint16 `json:"rows"`
			}
			if json.Unmarshal(data, &resize) == nil && resize.Type == "resize" {
				if err := s.writeAgent(a, protocol.Message{Type: "resize", ID: id, Cols: resize.Cols, Rows: resize.Rows}); err != nil {
					return
				}
			}
		}
	}
}

func (s *Server) writeAgent(a *agentConn, m protocol.Message) error {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	return a.ws.WriteJSON(m)
}

func (s *Server) writeBrowser(id string, messageType int, data []byte) {
	s.mu.RLock()
	b := s.routes[id]
	s.mu.RUnlock()
	if b == nil {
		return
	}
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	b.ws.WriteMessage(messageType, data)
}

func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
