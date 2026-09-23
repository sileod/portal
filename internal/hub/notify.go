package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sileod/portal/internal/protocol"
)

// NotifyConfig says where and when the hub pushes ntfy notifications about
// terminal state changes. The hub sends them itself, so they arrive even when
// no browser has Portal open.
type NotifyConfig struct {
	Server string `json:"server"`
	Topic  string `json:"topic"`
	// Waiting notifies when a terminal starts showing a prompt for the user.
	Waiting bool `json:"waiting"`
	// Finished notifies when a terminal that kept working for at least
	// MinWorkSeconds goes quiet.
	Finished bool `json:"finished"`
	// Idle notifies once a terminal that finished such a run has stayed
	// untouched for IdleMinutes.
	Idle bool `json:"idle"`
	// Hosts notifies when a machine disconnects or reconnects.
	Hosts          bool `json:"hosts"`
	MinWorkSeconds int  `json:"min_work_seconds"`
	IdleMinutes    int  `json:"idle_minutes"`
	// BaseURL is the Portal URL notifications link back to.
	BaseURL string `json:"base_url,omitempty"`
}

const defaultNtfyServer = "https://ntfy.sh"

var topicPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func defaultNotifyConfig() NotifyConfig {
	return NotifyConfig{Server: defaultNtfyServer, Waiting: true, Finished: true, MinWorkSeconds: 30, IdleMinutes: 10}
}

func (c *NotifyConfig) normalize() error {
	c.Server = strings.TrimRight(strings.TrimSpace(c.Server), "/")
	if c.Server == "" {
		c.Server = defaultNtfyServer
	}
	u, err := url.Parse(c.Server)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return errors.New("ntfy server must be an http(s) URL")
	}
	c.Topic = strings.TrimSpace(c.Topic)
	if c.Topic != "" && !topicPattern.MatchString(c.Topic) {
		return errors.New("topic may only use letters, digits, - and _")
	}
	if c.MinWorkSeconds < 0 || c.MinWorkSeconds > 3600 {
		return errors.New("minimum work time must be between 0 and 3600 seconds")
	}
	if c.IdleMinutes < 1 || c.IdleMinutes > 1440 {
		return errors.New("idle time must be between 1 and 1440 minutes")
	}
	if c.BaseURL != "" {
		if u, err := url.Parse(c.BaseURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			c.BaseURL = ""
		}
	}
	return nil
}

type notification struct {
	Title   string
	Message string
	Tags    string
	Host    string
	Session string
}

type sessionWatch struct {
	state        string
	workingSince time.Time
	// ranAt is when a long enough run ended; zero once its idle notice went out.
	ranAt time.Time
}

// notifier turns session snapshots into notifications.
type notifier struct {
	mu      sync.Mutex
	path    string
	config  NotifyConfig
	watches map[string]map[string]*sessionWatch // host -> session
	// lost holds hosts that disconnected, so only their return is announced.
	lost map[string]bool
	send func(NotifyConfig, notification) error
	now  func() time.Time
	// queue keeps notifications in order and off the callers' goroutines.
	queue chan queuedNotification

	lastSent  time.Time
	lastError string
}

func newNotifier() *notifier {
	n := &notifier{config: defaultNotifyConfig(), watches: map[string]map[string]*sessionWatch{}, lost: map[string]bool{}, now: time.Now}
	n.send = postNtfy
	n.queue = make(chan queuedNotification, 64)
	go func() {
		for q := range n.queue {
			n.record(q.send(q.config, q.msg))
		}
	}()
	return n
}

type queuedNotification struct {
	config NotifyConfig
	msg    notification
	send   func(NotifyConfig, notification) error
}

var ntfyClient = &http.Client{Timeout: 10 * time.Second}

func postNtfy(c NotifyConfig, n notification) error {
	req, err := http.NewRequest(http.MethodPost, c.Server+"/"+c.Topic, strings.NewReader(n.Message))
	if err != nil {
		return err
	}
	req.Header.Set("Title", n.Title)
	if n.Tags != "" {
		req.Header.Set("Tags", n.Tags)
	}
	if c.BaseURL != "" {
		click := c.BaseURL + "/"
		if n.Session != "" {
			click += "?" + url.Values{"host": {n.Host}, "session": {n.Session}}.Encode()
		}
		req.Header.Set("Click", click)
	}
	resp, err := ntfyClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy answered %s", resp.Status)
	}
	return nil
}

// load reads the saved config at path, keeping defaults when there is none.
func (n *notifier) load(path string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.path = path
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	config := defaultNotifyConfig()
	if err := json.Unmarshal(data, &config); err != nil || config.normalize() != nil {
		log.Printf("ignoring unreadable notification settings %s", path)
		return nil
	}
	n.config = config
	return nil
}

func (n *notifier) setConfig(c NotifyConfig) error {
	if err := c.normalize(); err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.config = c
	n.lastError = ""
	if n.path == "" {
		return nil
	}
	data, _ := json.MarshalIndent(c, "", "  ")
	tmp := filepath.Join(filepath.Dir(n.path), "."+filepath.Base(n.path)+".tmp")
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, n.path)
}

type notifyStatus struct {
	NotifyConfig
	LastSent  int64  `json:"last_sent,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

func (n *notifier) status() notifyStatus {
	n.mu.Lock()
	defer n.mu.Unlock()
	st := notifyStatus{NotifyConfig: n.config, LastError: n.lastError}
	if !n.lastSent.IsZero() {
		st.LastSent = n.lastSent.Unix()
	}
	return st
}

// observe compares a host's new sessions with what was seen before. The first
// snapshot of a session only records it, so hub restarts stay silent.
func (n *notifier) observe(host string, sessions []protocol.Session) {
	n.mu.Lock()
	defer n.mu.Unlock()
	now := n.now()
	c := n.config
	minWork := time.Duration(c.MinWorkSeconds) * time.Second
	prev := n.watches[host]
	next := make(map[string]*sessionWatch, len(sessions))
	var out []notification
	for _, s := range sessions {
		w, ok := prev[s.Session]
		if !ok {
			w = &sessionWatch{state: s.State}
			if s.State == "working" {
				w.workingSince = now
			}
			next[s.Session] = w
			continue
		}
		next[s.Session] = w
		if s.State == w.state {
			continue
		}
		if s.State == "working" {
			w.workingSince = now
			w.ranAt = time.Time{}
		}
		if w.state == "working" && s.State != "working" && now.Sub(w.workingSince) >= minWork {
			w.ranAt = now
			if s.State == "" && c.Finished {
				out = append(out, notification{Title: s.Session + " finished", Message: fmt.Sprintf("%s:%s stopped after %s of activity", host, s.Session, roundDuration(now.Sub(w.workingSince))), Tags: "white_check_mark", Host: host, Session: s.Session})
			}
		}
		if s.State == "waiting" && c.Waiting {
			out = append(out, notification{Title: s.Session + " needs input", Message: host + ":" + s.Session + " is waiting for you", Tags: "question", Host: host, Session: s.Session})
		}
		w.state = s.State
	}
	n.watches[host] = next
	n.dispatchLocked(out)
}

// tick sends idle notices for sessions that stayed quiet after a run.
func (n *notifier) tick() {
	n.mu.Lock()
	defer n.mu.Unlock()
	now := n.now()
	idle := time.Duration(n.config.IdleMinutes) * time.Minute
	var out []notification
	for host, sessions := range n.watches {
		for name, w := range sessions {
			if w.ranAt.IsZero() || w.state == "working" || now.Sub(w.ranAt) < idle {
				continue
			}
			w.ranAt = time.Time{}
			if n.config.Idle {
				out = append(out, notification{Title: name + " is idle", Message: fmt.Sprintf("%s:%s has been quiet for %s", host, name, roundDuration(idle)), Tags: "zzz", Host: host, Session: name})
			}
		}
	}
	n.dispatchLocked(out)
}

func (n *notifier) hostEvent(host string, connected bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	wasLost := n.lost[host]
	if connected {
		delete(n.lost, host)
	} else {
		n.lost[host] = true
	}
	if !n.config.Hosts {
		return
	}
	if connected {
		if !wasLost {
			return
		}
		n.dispatchLocked([]notification{{Title: host + " connected", Message: "Machine " + host + " is back on Portal", Tags: "electric_plug"}})
	} else {
		n.dispatchLocked([]notification{{Title: host + " disconnected", Message: "Machine " + host + " left Portal", Tags: "warning"}})
	}
}

func (n *notifier) test() error {
	n.mu.Lock()
	c := n.config
	send := n.send
	n.mu.Unlock()
	if c.Topic == "" {
		return errors.New("set a topic first")
	}
	err := send(c, notification{Title: "Portal test", Message: "Notifications from Portal reach this topic", Tags: "tada"})
	n.record(err)
	return err
}

func (n *notifier) dispatchLocked(out []notification) {
	c := n.config
	if c.Topic == "" || len(out) == 0 {
		return
	}
	for _, msg := range out {
		select {
		case n.queue <- queuedNotification{config: c, msg: msg, send: n.send}:
		default:
			log.Printf("ntfy: dropping %q, too many pending notifications", msg.Title)
		}
	}
}

func (n *notifier) record(err error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if err != nil {
		n.lastError = err.Error()
		log.Printf("ntfy: %v", err)
		return
	}
	n.lastSent = n.now()
	n.lastError = ""
}

func roundDuration(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	return strings.TrimSuffix(d.Round(time.Minute).String(), "0s")
}

// PersistNotify loads notification settings from path and saves changes there.
func (s *Server) PersistNotify(path string) error {
	return s.notify.load(path)
}

type notifyRequest struct {
	NotifyConfig
	Test bool `json:"test,omitempty"`
}

func (s *Server) handleNotify(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
	case http.MethodPost:
		if !sameOrigin(r) {
			http.Error(w, "bad origin", http.StatusForbidden)
			return
		}
		var req notifyRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := s.notify.setConfig(req.NotifyConfig); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Test {
			if err := s.notify.test(); err != nil {
				http.Error(w, "test notification failed: "+err.Error(), http.StatusBadGateway)
				return
			}
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.notify.status())
}
