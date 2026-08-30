package agent

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"github.com/sileod/portal/internal/protocol"
)

const (
	controlCapability   = "controls-v1"
	scheduleIndexOption = "@portal_schedule_ids"
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
	ws         *websocket.Conn
	writeMu    sync.Mutex
	termMu     sync.Mutex
	scheduleMu sync.Mutex
	terminals  map[string]*terminal
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
	hello := snapshotMessage("hello")
	hello.Host = cfg.Host
	hello.Capabilities = []string{controlCapability}
	if err := c.write(hello); err != nil {
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
			c.finishAction(m.ID, c.killSession(m.Session), true)
		case "rename_session":
			c.finishAction(m.ID, c.renameSession(m.Session, m.Name), true)
		case "create_session":
			c.finishAction(m.ID, c.createSession(m.Name, m.Command), true)
		case "schedule_input":
			c.finishAction(m.ID, c.scheduleInput(m.Session, m.Text, m.DelaySeconds, m.Repeat, m.IntervalSeconds), true)
		case "status_color":
			c.finishAction(m.ID, c.setStatusColor(m.Value), false)
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

func (c *connection) finishAction(id string, err error, publish bool) {
	if err == nil && publish {
		_ = c.write(snapshotMessage("sessions"))
	}
	if id == "" {
		return
	}
	result := protocol.Message{Type: "action_result", ID: id}
	if err != nil {
		result.Error = err.Error()
	}
	_ = c.write(result)
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
			m := snapshotMessage("sessions")
			data, _ := json.Marshal(struct {
				Sessions  []protocol.Session  `json:"sessions"`
				Schedules []protocol.Schedule `json:"schedules"`
			}{m.SessionInfos, m.Schedules})
			joined := string(data)
			if joined != last {
				if c.write(m) != nil {
					return
				}
				last = joined
			}
		}
	}
}

func snapshotMessage(messageType string) protocol.Message {
	names, infos := sessionInfos()
	return protocol.Message{Type: messageType, Sessions: names, SessionInfos: infos, Schedules: pendingSchedules()}
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

func (c *connection) killSession(session string) error {
	if !contains(Sessions(), session) {
		return errors.New("Portal session not found")
	}
	out, err := exec.Command("tmux", "kill-session", "-t", session).CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux kill-session: %s", commandError(err, out))
	}
	return nil
}

func (c *connection) renameSession(session, name string) error {
	if !contains(Sessions(), session) {
		return errors.New("Portal session not found")
	}
	if !validSessionName(name) {
		return errors.New("invalid session name")
	}
	if tmuxSessionExists(name) {
		return errors.New("tmux session already exists: " + name)
	}
	out, err := exec.Command("tmux", "rename-session", "-t", session, name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux rename-session: %s", commandError(err, out))
	}
	return nil
}

func (c *connection) createSession(name, command string) error {
	if !validSessionName(name) {
		return errors.New("invalid session name")
	}
	if tmuxSessionExists(name) {
		return errors.New("tmux session already exists: " + name)
	}
	args := []string{"new-session", "-d", "-s", name}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		args = append(args, "-c", home)
	}
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux new-session: %s", commandError(err, out))
	}
	if out, err := exec.Command("tmux", "set-option", "-t", name, "@portal", "1").CombinedOutput(); err != nil {
		_ = exec.Command("tmux", "kill-session", "-t", name).Run()
		return fmt.Errorf("mark Portal session: %s", commandError(err, out))
	}
	if err := applyPortalStatusColor(name); err != nil {
		log.Printf("apply status color to %s: %v", name, err)
	}
	if command != "" {
		if strings.ContainsAny(command, "\r\n") {
			return errors.New("command must be one line")
		}
		if out, err := exec.Command("tmux", "send-keys", "-t", name, "-l", "--", command).CombinedOutput(); err != nil {
			return fmt.Errorf("tmux send command: %s", commandError(err, out))
		}
		if out, err := exec.Command("tmux", "send-keys", "-t", name, "Enter").CombinedOutput(); err != nil {
			return fmt.Errorf("tmux send Enter: %s", commandError(err, out))
		}
	}
	return nil
}

func (c *connection) scheduleInput(session, text string, delaySeconds int64, repeat int, intervalSeconds int64) error {
	if delaySeconds <= 0 || text == "" || strings.ContainsAny(text, "\r\n") || !contains(Sessions(), session) {
		return errors.New("invalid scheduled input")
	}
	if repeat <= 0 {
		repeat = 1
	}
	if repeat > 100 || (repeat > 1 && intervalSeconds <= 0) {
		return errors.New("invalid repeat settings")
	}
	paneOut, err := exec.Command("tmux", "display-message", "-p", "-t", session, "#{pane_id}").Output()
	if err != nil {
		return fmt.Errorf("resolve tmux pane: %w", err)
	}
	pane := strings.TrimSpace(string(paneOut))
	if pane == "" {
		return errors.New("could not resolve tmux pane")
	}
	now := time.Now().Unix()
	id := fmt.Sprintf("%x", time.Now().UnixNano())
	option := scheduleOption(id)
	schedule := protocol.Schedule{
		ID:              id,
		Session:         session,
		Pane:            pane,
		Text:            text,
		CreatedAt:       now,
		FirstAt:         now + delaySeconds,
		Repeat:          repeat,
		IntervalSeconds: intervalSeconds,
	}
	if err := c.saveSchedule(schedule); err != nil {
		return err
	}
	script := fmt.Sprintf(
		"sleep %d; i=1; while [ $i -le %d ]; do tmux send-keys -t %s -l -- %s && sleep 0.2 && tmux send-keys -t %s Enter || break; i=$((i+1)); if [ $i -le %d ]; then sleep %d; fi; done; tmux set-option -gu %s",
		delaySeconds,
		repeat,
		shellQuote(pane),
		shellQuote(text),
		shellQuote(pane),
		repeat,
		intervalSeconds,
		shellQuote(option),
	)
	out, err := exec.Command("tmux", "run-shell", "-b", script).CombinedOutput()
	if err != nil {
		_ = exec.Command("tmux", "set-option", "-gu", option).Run()
		return fmt.Errorf("tmux schedule: %s", commandError(err, out))
	}
	return nil
}

func (c *connection) saveSchedule(schedule protocol.Schedule) error {
	data, err := json.Marshal(schedule)
	if err != nil {
		return err
	}
	option := scheduleOption(schedule.ID)
	c.scheduleMu.Lock()
	defer c.scheduleMu.Unlock()
	if out, err := exec.Command("tmux", "set-option", "-g", option, string(data)).CombinedOutput(); err != nil {
		return fmt.Errorf("save scheduled input: %s", commandError(err, out))
	}
	ids := scheduleIDs()
	if !contains(ids, schedule.ID) {
		ids = append(ids, schedule.ID)
	}
	if err := writeScheduleIDs(ids); err != nil {
		_ = exec.Command("tmux", "set-option", "-gu", option).Run()
		return err
	}
	return nil
}

func (c *connection) setStatusColor(value string) error {
	if !validHexColor(value) {
		return errors.New("invalid status color")
	}
	if out, err := exec.Command("tmux", "set-option", "-g", "@portal_status_bg", value).CombinedOutput(); err != nil {
		return fmt.Errorf("save tmux status color: %s", commandError(err, out))
	}
	for _, session := range Sessions() {
		if err := applyPortalStatusColor(session); err != nil {
			return fmt.Errorf("apply status color to %s: %w", session, err)
		}
	}
	return nil
}

func applyPortalStatusColor(session string) error {
	out, err := exec.Command("tmux", "show-options", "-gqv", "@portal_status_bg").Output()
	if err != nil {
		return nil
	}
	color := strings.TrimSpace(string(out))
	if !validHexColor(color) {
		return nil
	}
	style := "bg=" + color
	for _, option := range []string{"status-style", "status-left-style", "status-right-style", "window-status-style", "window-status-current-style"} {
		if out, err := exec.Command("tmux", "set-option", "-t", session, option, style).CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %s", option, commandError(err, out))
		}
	}
	_ = exec.Command("tmux", "set-option", "-t", session, "status-bg", color).Run()
	return nil
}

func tmuxSessionExists(name string) bool {
	return exec.Command("tmux", "has-session", "-t", name).Run() == nil
}

func commandError(err error, out []byte) string {
	text := strings.TrimSpace(string(out))
	if text != "" {
		return text
	}
	return err.Error()
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

func sessionInfos() ([]string, []protocol.Session) {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}\t#{@portal}").Output()
	if err != nil {
		return nil, nil
	}
	managed := map[string]bool{}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 && parts[1] == "1" && parts[0] != "" {
			managed[parts[0]] = true
			names = append(names, parts[0])
		}
	}
	activity := map[string]int64{}
	if windows, err := exec.Command("tmux", "list-windows", "-a", "-F", "#{session_name}\t#{window_activity}").Output(); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(windows)), "\n") {
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) != 2 || !managed[parts[0]] {
				continue
			}
			ts, _ := strconv.ParseInt(parts[1], 10, 64)
			if ts > activity[parts[0]] {
				activity[parts[0]] = ts
			}
		}
	}
	var infos []protocol.Session
	for _, name := range names {
		infos = append(infos, protocol.Session{Session: name, LastActivity: activity[name]})
	}
	sort.Strings(names)
	sort.Slice(infos, func(i, j int) bool { return infos[i].Session < infos[j].Session })
	return names, infos
}

func scheduleOption(id string) string {
	return "@portal_schedule_" + id
}

func validScheduleID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

func scheduleIDs() []string {
	out, err := exec.Command("tmux", "show-options", "-gqv", scheduleIndexOption).Output()
	if err != nil {
		return nil
	}
	var ids []string
	seen := map[string]bool{}
	for _, id := range strings.FieldsFunc(strings.TrimSpace(string(out)), func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	}) {
		if validScheduleID(id) && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

func writeScheduleIDs(ids []string) error {
	clean := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if validScheduleID(id) && !seen[id] {
			seen[id] = true
			clean = append(clean, id)
		}
	}
	if len(clean) == 0 {
		_ = exec.Command("tmux", "set-option", "-gu", scheduleIndexOption).Run()
		return nil
	}
	if out, err := exec.Command("tmux", "set-option", "-g", scheduleIndexOption, strings.Join(clean, ",")).CombinedOutput(); err != nil {
		return fmt.Errorf("save schedule index: %s", commandError(err, out))
	}
	return nil
}

func pendingSchedules() []protocol.Schedule {
	ids := scheduleIDs()
	if len(ids) == 0 {
		return nil
	}
	var schedules []protocol.Schedule
	live := make([]string, 0, len(ids))
	for _, id := range ids {
		option := scheduleOption(id)
		raw, err := exec.Command("tmux", "show-options", "-gqv", option).Output()
		if err != nil {
			continue
		}
		var schedule protocol.Schedule
		if json.Unmarshal([]byte(strings.TrimSpace(string(raw))), &schedule) != nil || schedule.ID != id || schedule.Pane == "" {
			continue
		}
		current, paneErr := exec.Command("tmux", "display-message", "-p", "-t", schedule.Pane, "#{session_name}").Output()
		if paneErr != nil {
			_ = exec.Command("tmux", "set-option", "-gu", option).Run()
			continue
		}
		schedule.Session = strings.TrimSpace(string(current))
		live = append(live, id)
		schedules = append(schedules, schedule)
	}
	if len(live) != len(ids) {
		_ = writeScheduleIDs(live)
	}
	sort.Slice(schedules, func(i, j int) bool {
		if schedules[i].FirstAt == schedules[j].FirstAt {
			return schedules[i].ID < schedules[j].ID
		}
		return schedules[i].FirstAt < schedules[j].FirstAt
	})
	return schedules
}

func Sessions() []string {
	names, _ := sessionInfos()
	return names
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
