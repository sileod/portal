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
	cmd := exec.Command("tmux", "attach-session", "-t", session)
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
