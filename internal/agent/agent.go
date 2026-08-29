package agent

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"github.com/sileod/portal/internal/protocol"
)

type Config struct {
	URL   string
	Token string
	Host  string
}

type terminal struct {
	pty *os.File
	cmd *exec.Cmd
}

type connection struct {
	ws        *websocket.Conn
	writeMu   sync.Mutex
	termMu    sync.Mutex
	terminals map[string]*terminal
}

func Run(cfg Config) error {
	if cfg.URL == "" || cfg.Token == "" || cfg.Host == "" {
		return fmt.Errorf("portal URL, token, and host are required")
	}
	backoff := time.Second
	for {
		err := runOnce(cfg)
		if err != nil {
			log.Printf("hub connection: %v", err)
		}
		time.Sleep(backoff)
		if backoff < 15*time.Second {
			backoff *= 2
		}
	}
}

func runOnce(cfg Config) error {
	endpoint, err := websocketURL(cfg.URL, "/api/agent")
	if err != nil {
		return err
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+cfg.Token)
	ws, _, err := websocket.DefaultDialer.Dial(endpoint, header)
	if err != nil {
		return err
	}
	c := &connection{ws: ws, terminals: map[string]*terminal{}}
	defer func() {
		c.closeAll()
		ws.Close()
	}()
	if err := c.write(protocol.Message{Type: "hello", Host: cfg.Host, Sessions: Sessions()}); err != nil {
		return err
	}
	log.Printf("connected to %s as %s", cfg.URL, cfg.Host)
	done := make(chan struct{})
	defer close(done)
	go c.publishSessions(done)
	for {
		var m protocol.Message
		if err := ws.ReadJSON(&m); err != nil {
			return err
		}
		switch m.Type {
		case "open":
			c.open(m.ID, m.Session)
		case "input":
			c.input(m.ID, m.Data)
		case "resize":
			c.resize(m.ID, m.Cols, m.Rows)
		case "close":
			c.close(m.ID)
		case "kill_session":
			c.killSession(m.Session)
		case "rename_session":
			c.renameSession(m.Session, m.Name)
		case "create_session":
			c.createSession(m.Name, m.Command)
		case "schedule_input":
			c.scheduleInput(m.Session, m.Text, m.DelaySeconds, m.Repeat, m.IntervalSeconds)
		case "status_color":
			c.setStatusColor(m.Value)
		}
	}
}

func websocketURL(base, path string) (string, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("unsupported portal URL scheme %q", u.Scheme)
	}
	u.Path = path
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func (c *connection) write(m protocol.Message) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.ws.WriteJSON(m)
}

func (c *connection) publishSessions(done <-chan struct{}) {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	last := ""
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			sessions := Sessions()
			joined := strings.Join(sessions, "\x00")
			if joined != last {
				if c.write(protocol.Message{Type: "sessions", Sessions: sessions}) != nil {
					return
				}
				last = joined
			}
		}
	}
}

func (c *connection) open(id, session string) {
	if id == "" || session == "" {
		return
	}
	if !contains(Sessions(), session) {
		c.write(protocol.Message{Type: "error", ID: id, Error: "portal session not found: " + session})
		return
	}
	c.close(id)
	cmd := exec.Command("tmux", "attach-session", "-t", "="+session)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 120, Rows: 32})
	if err != nil {
		c.write(protocol.Message{Type: "error", ID: id, Error: err.Error()})
		return
	}
	t := &terminal{pty: ptmx, cmd: cmd}
	c.termMu.Lock()
	c.terminals[id] = t
	c.termMu.Unlock()
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if werr := c.write(protocol.Message{Type: "output", ID: id, Data: base64.StdEncoding.EncodeToString(buf[:n])}); werr != nil {
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					c.write(protocol.Message{Type: "error", ID: id, Error: err.Error()})
				}
				return
			}
		}
	}()
}

func (c *connection) input(id, encoded string) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return
	}
	c.termMu.Lock()
	t := c.terminals[id]
	c.termMu.Unlock()
	if t != nil {
		t.pty.Write(data)
	}
}

func (c *connection) resize(id string, cols, rows uint16) {
	if cols == 0 || rows == 0 {
		return
	}
	c.termMu.Lock()
	t := c.terminals[id]
	c.termMu.Unlock()
	if t != nil {
		pty.Setsize(t.pty, &pty.Winsize{Cols: cols, Rows: rows})
	}
}

func (c *connection) close(id string) {
	c.termMu.Lock()
	t := c.terminals[id]
	delete(c.terminals, id)
	c.termMu.Unlock()
	if t == nil {
		return
	}
	t.pty.Close()
	if t.cmd.Process != nil {
		t.cmd.Process.Kill()
		t.cmd.Wait()
	}
}

func (c *connection) closeAll() {
	c.termMu.Lock()
	ids := make([]string, 0, len(c.terminals))
	for id := range c.terminals {
		ids = append(ids, id)
	}
	c.termMu.Unlock()
	for _, id := range ids {
		c.close(id)
	}
}

func (c *connection) killSession(session string) {
	if !contains(Sessions(), session) {
		return
	}
	if out, err := exec.Command("tmux", "kill-session", "-t", "="+session).CombinedOutput(); err != nil {
		log.Printf("kill session %s: %v: %s", session, err, strings.TrimSpace(string(out)))
	}
}

func (c *connection) renameSession(session, name string) {
	if !contains(Sessions(), session) || !validSessionName(name) || contains(Sessions(), name) {
		return
	}
	if out, err := exec.Command("tmux", "rename-session", "-t", "="+session, name).CombinedOutput(); err != nil {
		log.Printf("rename session %s: %v: %s", session, err, strings.TrimSpace(string(out)))
	}
}

func (c *connection) createSession(name, command string) {
	if !validSessionName(name) || contains(Sessions(), name) {
		return
	}
	args := []string{"new-session", "-d", "-s", name}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		args = append(args, "-c", home)
	}
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		log.Printf("create session %s: %v: %s", name, err, strings.TrimSpace(string(out)))
		return
	}
	exec.Command("tmux", "set-option", "-t", "="+name, "@portal", "1").Run()
	applyPortalStatusColor(name)
	if command != "" && !strings.ContainsAny(command, "\r\n") {
		exec.Command("tmux", "send-keys", "-t", "="+name, "-l", "--", command).Run()
		exec.Command("tmux", "send-keys", "-t", "="+name, "Enter").Run()
	}
}

func (c *connection) scheduleInput(session, text string, delaySeconds int64, repeat int, intervalSeconds int64) {
	if delaySeconds <= 0 || text == "" || strings.ContainsAny(text, "\r\n") || !contains(Sessions(), session) {
		return
	}
	if repeat <= 0 {
		repeat = 1
	}
	if repeat > 100 {
		return
	}
	if repeat > 1 && intervalSeconds <= 0 {
		return
	}
	paneOut, err := exec.Command("tmux", "display-message", "-p", "-t", "="+session, "#{pane_id}").Output()
	if err != nil {
		log.Printf("schedule input for %s: could not resolve pane: %v", session, err)
		return
	}
	pane := strings.TrimSpace(string(paneOut))
	if pane == "" {
		return
	}
	script := fmt.Sprintf(
		"sleep %d; i=1; while [ $i -le %d ]; do tmux send-keys -t %s -l -- %s; tmux send-keys -t %s Enter; i=$((i+1)); if [ $i -le %d ]; then sleep %d; fi; done",
		delaySeconds,
		repeat,
		shellQuote(pane),
		shellQuote(text),
		shellQuote(pane),
		repeat,
		intervalSeconds,
	)
	if out, err := exec.Command("tmux", "run-shell", "-b", script).CombinedOutput(); err != nil {
		log.Printf("schedule input for %s: %v: %s", session, err, strings.TrimSpace(string(out)))
	}
}

func (c *connection) setStatusColor(value string) {
	if !validHexColor(value) {
		return
	}
	exec.Command("tmux", "set-option", "-g", "@portal_status_bg", value).Run()
	for _, session := range Sessions() {
		applyPortalStatusColor(session)
	}
}

func applyPortalStatusColor(session string) {
	out, err := exec.Command("tmux", "show-option", "-gqv", "@portal_status_bg").Output()
	if err != nil {
		return
	}
	color := strings.TrimSpace(string(out))
	if !validHexColor(color) {
		return
	}
	exec.Command("tmux", "set-option", "-t", "="+session, "status-style", "bg="+color).Run()
}

func validHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, r := range s[1:] {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
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

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func Sessions() []string {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}\t#{@portal}").Output()
	if err != nil {
		return nil
	}
	var sessions []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 && parts[1] == "1" && parts[0] != "" {
			sessions = append(sessions, parts[0])
		}
	}
	sort.Strings(sessions)
	return sessions
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
