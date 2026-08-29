package hub

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sileod/portal/internal/auth"
	"github.com/sileod/portal/internal/protocol"
	"github.com/sileod/portal/internal/webui"
)

const controlCapability = "controls-v1"

const (
	browserSessionTTL = 30 * 24 * time.Hour
	authAttemptTTL     = 24 * time.Hour
	maxAuthDelay       = 5 * time.Minute
)

type agentConn struct {
	host         string
	ws           *websocket.Conn
	writeMu      sync.Mutex
	sessions     []string
	sessionInfos []protocol.Session
	schedules    []protocol.Schedule
	capabilities []string
}

type browserConn struct {
	host    string
	ws      *websocket.Conn
	writeMu sync.Mutex
}

type pendingAction struct {
	host string
	ch   chan protocol.Message
}

type authAttempt struct {
	failures int
	next     time.Time
	last     time.Time
}

type Server struct {
	passwordHash string
	agentToken   string
	mu           sync.RWMutex
	agents       map[string]*agentConn
	routes       map[string]*browserConn
	pending      map[string]pendingAction
	authMu       sync.Mutex
	sessions     map[string]time.Time
	attempts     map[string]authAttempt
	upgrader     websocket.Upgrader
}

func New(passwordHash, agentToken string) *Server {
	return &Server{
		passwordHash: passwordHash,
		agentToken:   agentToken,
		agents:       map[string]*agentConn{},
		routes:       map[string]*browserConn{},
		pending:      map[string]pendingAction{},
		sessions:     map[string]time.Time{},
		attempts:     map[string]authAttempt{},
		upgrader:     websocket.Upgrader{CheckOrigin: sameOrigin},
	}
}

func (s *Server) Run(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/api/enroll", s.handleEnroll)
	mux.HandleFunc("/api/sessions", s.requireBrowser(s.handleSessions))
	mux.HandleFunc("/api/session", s.requireBrowser(s.handleSessionAction))
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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; connect-src 'self' ws: wss:; font-src 'self' data:; img-src 'self' data:")
		next.ServeHTTP(w, r)
	})
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

func (s *Server) browserOK(r *http.Request) bool {
	c, err := r.Cookie("portal_session")
	if err != nil || c.Value == "" {
		return false
	}
	now := time.Now()
	s.authMu.Lock()
	expires, ok := s.sessions[c.Value]
	if ok && !expires.After(now) {
		delete(s.sessions, c.Value)
		ok = false
	}
	s.authMu.Unlock()
	return ok
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
		ip := clientIP(r)
		if wait := s.authWait(ip); wait > 0 {
			writeThrottle(w, wait, true)
			return
		}
		if err := r.ParseForm(); err != nil || !auth.VerifyPassword(s.passwordHash, r.FormValue("password")) {
			wait := s.authFailure(ip)
			w.Header().Set("Retry-After", strconv.Itoa(retrySeconds(wait)))
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, loginPage("wrong password"))
			return
		}
		s.authSuccess(ip)
		token, err := auth.RandomToken()
		if err != nil {
			http.Error(w, "could not create session", http.StatusInternalServerError)
			return
		}
		s.authMu.Lock()
		s.sessions[token] = time.Now().Add(browserSessionTTL)
		s.authMu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "portal_session", Value: token, Path: "/", HttpOnly: true, Secure: isSecure(r), SameSite: http.SameSiteStrictMode, MaxAge: int(browserSessionTTL / time.Second)})
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

func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ip := clientIP(r)
	if wait := s.authWait(ip); wait > 0 {
		writeThrottle(w, wait, false)
		return
	}
	if err := r.ParseForm(); err != nil || !auth.VerifyPassword(s.passwordHash, r.FormValue("password")) {
		wait := s.authFailure(ip)
		w.Header().Set("Retry-After", strconv.Itoa(retrySeconds(wait)))
		http.Error(w, "wrong password", http.StatusUnauthorized)
		return
	}
	s.authSuccess(ip)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Token string `json:"token"`
	}{Token: s.agentToken})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("portal_session"); err == nil && c.Value != "" {
		s.authMu.Lock()
		delete(s.sessions, c.Value)
		s.authMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "portal_session", Value: "", Path: "/", HttpOnly: true, Secure: isSecure(r), SameSite: http.SameSiteStrictMode, MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func loginPage(errText string) string {
	errHTML := ""
	if errText != "" {
		errHTML = `<p class="error">` + errText + `</p>`
	}
	return `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="theme-color" content="#111111"><link rel="icon" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 120 120'><text x='60' y='86' text-anchor='middle' font-size='78'>🌀</text></svg>"><title>Portal</title><style>:root{color-scheme:light dark}*{box-sizing:border-box}body{margin:0;height:100vh;display:grid;place-items:center;font:14px ui-monospace,SFMono-Regular,Menlo,monospace}form{width:min(360px,calc(100vw - 40px))}h1{font-size:20px}input,button{width:100%;padding:12px;font:inherit;margin-top:8px}button{cursor:pointer}.error{color:#c33}</style></head><body><form method="post"><h1>🌀 Portal</h1><input name="password" type="password" autocomplete="current-password" autofocus placeholder="password"><button>Enter</button>` + errHTML + `</form></body></html>`
}

func (s *Server) authWait(ip string) time.Duration {
	now := time.Now()
	s.authMu.Lock()
	defer s.authMu.Unlock()
	s.pruneAttemptsLocked(now)
	a, ok := s.attempts[ip]
	if !ok || !a.next.After(now) {
		return 0
	}
	return a.next.Sub(now)
}

func (s *Server) authFailure(ip string) time.Duration {
	now := time.Now()
	s.authMu.Lock()
	defer s.authMu.Unlock()
	s.pruneAttemptsLocked(now)
	a := s.attempts[ip]
	a.failures++
	shift := a.failures - 1
	if shift > 8 {
		shift = 8
	}
	delay := time.Second * time.Duration(1<<shift)
	if delay > maxAuthDelay {
		delay = maxAuthDelay
	}
	a.next = now.Add(delay)
	a.last = now
	s.attempts[ip] = a
	return delay
}

func (s *Server) authSuccess(ip string) {
	s.authMu.Lock()
	delete(s.attempts, ip)
	s.authMu.Unlock()
}

func (s *Server) pruneAttemptsLocked(now time.Time) {
	if len(s.attempts) < 1024 {
		return
	}
	oldestIP := ""
	var oldest time.Time
	for ip, a := range s.attempts {
		if now.Sub(a.last) > authAttemptTTL {
			delete(s.attempts, ip)
			continue
		}
		if oldestIP == "" || a.last.Before(oldest) {
			oldestIP, oldest = ip, a.last
		}
	}
	if len(s.attempts) >= 8192 && oldestIP != "" {
		delete(s.attempts, oldestIP)
	}
}

func writeThrottle(w http.ResponseWriter, wait time.Duration, html bool) {
	seconds := retrySeconds(wait)
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	if html {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, loginPage(fmt.Sprintf("too many attempts; retry in %ds", seconds)))
		return
	}
	http.Error(w, fmt.Sprintf("too many attempts; retry in %ds", seconds), http.StatusTooManyRequests)
}

func retrySeconds(wait time.Duration) int {
	seconds := int((wait + time.Second - 1) / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remote := net.ParseIP(strings.TrimSpace(host))
	if remote != nil && remote.IsLoopback() {
		parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		for i := len(parts) - 1; i >= 0; i-- {
			if ip := net.ParseIP(strings.TrimSpace(parts[i])); ip != nil {
				return ip.String()
			}
		}
	}
	if remote != nil {
		return remote.String()
	}
	return host
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

func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	return strings.TrimPrefix(header, prefix), true
}

func (s *Server) handleSessions(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	list := make([]protocol.Session, 0)
	schedules := make([]protocol.Schedule, 0)
	hosts := make([]string, 0, len(s.agents))
	for host, a := range s.agents {
		hosts = append(hosts, host)
		if len(a.sessionInfos) > 0 {
			for _, session := range a.sessionInfos {
				session.Host = host
				list = append(list, session)
			}
		} else {
			for _, session := range a.sessions {
				list = append(list, protocol.Session{Host: host, Session: session})
			}
		}
		for _, schedule := range a.schedules {
			schedule.Host = host
			schedules = append(schedules, schedule)
		}
	}
	hostCount := len(s.agents)
	s.mu.RUnlock()
	sort.Strings(hosts)
	sort.Slice(list, func(i, j int) bool {
		if list[i].Host == list[j].Host {
			return list[i].Session < list[j].Session
		}
		return list[i].Host < list[j].Host
	})
	sort.Slice(schedules, func(i, j int) bool {
		if schedules[i].FirstAt == schedules[j].FirstAt {
			return schedules[i].ID < schedules[j].ID
		}
		return schedules[i].FirstAt < schedules[j].FirstAt
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(protocol.SessionList{HostCount: hostCount, Hosts: hosts, Sessions: list, Schedules: schedules})
}

type sessionActionRequest struct {
	Action   string `json:"action"`
	Host     string `json:"host,omitempty"`
	Session  string `json:"session,omitempty"`
	Name     string `json:"name,omitempty"`
	Command  string `json:"command,omitempty"`
	Delay    string `json:"delay,omitempty"`
	Text     string `json:"text,omitempty"`
	Repeat   int    `json:"repeat,omitempty"`
	Interval string `json:"interval,omitempty"`
	Value    string `json:"value,omitempty"`
}

func (s *Server) handleSessionAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "bad origin", http.StatusForbidden)
		return
	}
	var req sessionActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if req.Action == "status_color" {
		if !validHexColor(req.Value) {
			http.Error(w, "status color must be #RRGGBB", http.StatusBadRequest)
			return
		}
		s.mu.RLock()
		targets := make([]*agentConn, 0, len(s.agents))
		for host, a := range s.agents {
			if req.Host == "" || req.Host == host {
				targets = append(targets, a)
			}
		}
		s.mu.RUnlock()
		if len(targets) == 0 {
			http.Error(w, "host not found", http.StatusNotFound)
			return
		}
		for _, a := range targets {
			if !hasCapability(a, controlCapability) {
				http.Error(w, "update Portal on host "+a.host+" to use settings", http.StatusConflict)
				return
			}
			if err := s.callAgent(a, protocol.Message{Type: "status_color", Value: req.Value}); err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
		}
		writeOK(w)
		return
	}

	if req.Host == "" {
		http.Error(w, "host required", http.StatusBadRequest)
		return
	}
	s.mu.RLock()
	a := s.agents[req.Host]
	s.mu.RUnlock()
	if a == nil {
		http.Error(w, "host not found", http.StatusNotFound)
		return
	}
	if !hasCapability(a, controlCapability) {
		http.Error(w, "update Portal on host "+a.host+" to use this control", http.StatusConflict)
		return
	}

	if req.Action == "create" {
		if !validSessionName(req.Name) {
			http.Error(w, "invalid session name", http.StatusBadRequest)
			return
		}
		if req.Command != "" && (len(req.Command) > 4096 || strings.ContainsAny(req.Command, "\r\n")) {
			http.Error(w, "command must be one line up to 4096 bytes", http.StatusBadRequest)
			return
		}
		s.mu.RLock()
		exists := containsString(a.sessions, req.Name)
		s.mu.RUnlock()
		if exists {
			http.Error(w, "session already exists", http.StatusConflict)
			return
		}
		if err := s.callAgent(a, protocol.Message{Type: "create_session", Name: req.Name, Command: req.Command}); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeOK(w)
		return
	}

	if req.Session == "" {
		http.Error(w, "session required", http.StatusBadRequest)
		return
	}
	s.mu.RLock()
	found := containsString(a.sessions, req.Session)
	s.mu.RUnlock()
	if !found {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	var m protocol.Message
	switch req.Action {
	case "kill":
		m = protocol.Message{Type: "kill_session", Session: req.Session}
	case "rename":
		if !validSessionName(req.Name) {
			http.Error(w, "invalid session name", http.StatusBadRequest)
			return
		}
		s.mu.RLock()
		exists := containsString(a.sessions, req.Name)
		s.mu.RUnlock()
		if exists {
			http.Error(w, "session already exists", http.StatusConflict)
			return
		}
		m = protocol.Message{Type: "rename_session", Session: req.Session, Name: req.Name}
	case "schedule":
		delay, err := time.ParseDuration(strings.TrimSpace(req.Delay))
		if err != nil || delay < time.Second || delay > 30*24*time.Hour {
			http.Error(w, "delay must be between 1s and 720h", http.StatusBadRequest)
			return
		}
		if req.Text == "" || len(req.Text) > 4096 || strings.ContainsAny(req.Text, "\r\n") {
			http.Error(w, "message must be one non-empty line up to 4096 bytes", http.StatusBadRequest)
			return
		}
		repeat := req.Repeat
		if repeat == 0 {
			repeat = 1
		}
		if repeat < 1 || repeat > 100 {
			http.Error(w, "repeat must be between 1 and 100", http.StatusBadRequest)
			return
		}
		var interval time.Duration
		if repeat > 1 {
			interval, err = time.ParseDuration(strings.TrimSpace(req.Interval))
			if err != nil || interval < time.Second || interval > 30*24*time.Hour {
				http.Error(w, "repeat interval must be between 1s and 720h", http.StatusBadRequest)
				return
			}
		}
		m = protocol.Message{Type: "schedule_input", Session: req.Session, Text: req.Text, DelaySeconds: int64(delay / time.Second), Repeat: repeat, IntervalSeconds: int64(interval / time.Second)}
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}
	if err := s.callAgent(a, m); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeOK(w)
}

func writeOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"ok":true}`)
}

func validSessionName(name string) bool {
	if name == "" || len(name) > 80 {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func validHexColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, r := range value[1:] {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func hasCapability(a *agentConn, capability string) bool {
	return containsString(a.capabilities, capability)
}

func applyAgentSnapshot(a *agentConn, m protocol.Message) {
	a.sessions = append([]string(nil), m.Sessions...)
	a.sessionInfos = append([]protocol.Session(nil), m.SessionInfos...)
	a.schedules = append([]protocol.Schedule(nil), m.Schedules...)
}

func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	token, ok := bearerToken(r)
	if !ok || !constantEqual(token, s.agentToken) {
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
	a := &agentConn{host: hello.Host, ws: ws, capabilities: append([]string(nil), hello.Capabilities...)}
	applyAgentSnapshot(a, hello)
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
				applyAgentSnapshot(a, m)
			}
			s.mu.Unlock()
		case "action_result":
			s.resolveAction(m)
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

func (s *Server) callAgent(a *agentConn, m protocol.Message) error {
	id := randomID()
	m.ID = id
	ch := make(chan protocol.Message, 1)
	s.mu.Lock()
	s.pending[id] = pendingAction{host: a.host, ch: ch}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
	}()
	if err := s.writeAgent(a, m); err != nil {
		return fmt.Errorf("host %s disconnected", a.host)
	}
	select {
	case result := <-ch:
		if result.Error != "" {
			return fmt.Errorf("%s: %s", a.host, result.Error)
		}
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("host %s did not acknowledge action; update Portal on that host", a.host)
	}
}

func (s *Server) resolveAction(m protocol.Message) {
	s.mu.RLock()
	p, ok := s.pending[m.ID]
	s.mu.RUnlock()
	if !ok {
		return
	}
	select {
	case p.ch <- m:
	default:
	}
}

func (s *Server) removeAgent(a *agentConn) {
	s.mu.Lock()
	if s.agents[a.host] != a {
		s.mu.Unlock()
		return
	}
	delete(s.agents, a.host)
	var closeRoutes []*browserConn
	for id, b := range s.routes {
		if b.host == a.host {
			delete(s.routes, id)
			closeRoutes = append(closeRoutes, b)
		}
	}
	var pending []chan protocol.Message
	for _, p := range s.pending {
		if p.host == a.host {
			pending = append(pending, p.ch)
		}
	}
	s.mu.Unlock()
	for _, b := range closeRoutes {
		b.ws.Close()
	}
	for _, ch := range pending {
		select {
		case ch <- protocol.Message{Error: "host disconnected"}:
		default:
		}
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
