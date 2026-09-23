package hub

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/sileod/portal/internal/protocol"
)

type sentLog struct {
	mu     sync.Mutex
	titles []string
}

func (l *sentLog) get() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.titles...)
}

func testNotifier(c NotifyConfig) (*notifier, *sentLog, *time.Time) {
	n := newNotifier()
	n.config = c
	clock := time.Unix(1000, 0)
	n.now = func() time.Time { return clock }
	sent := &sentLog{}
	n.send = func(_ NotifyConfig, m notification) error {
		sent.mu.Lock()
		sent.titles = append(sent.titles, m.Title)
		sent.mu.Unlock()
		return nil
	}
	return n, sent, &clock
}

func waitSent(t *testing.T, sent *sentLog, want ...string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		got := sent.get()
		if len(got) == len(want) {
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("sent %q, want %q", got, want)
				}
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("sent %q, want %q", got, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func state(session, s string) []protocol.Session {
	return []protocol.Session{{Session: session, State: s}}
}

func TestNotifierEvents(t *testing.T) {
	c := defaultNotifyConfig()
	c.Topic = "portal-test"
	c.Idle = true
	n, sent, clock := testNotifier(c)

	n.observe("box", state("agent", "waiting")) // first sight stays silent
	n.observe("box", state("agent", "working"))
	*clock = clock.Add(5 * time.Second)
	n.observe("box", state("agent", "")) // too short a run
	n.observe("box", state("agent", "working"))
	*clock = clock.Add(time.Minute)
	n.observe("box", state("agent", ""))
	n.observe("box", state("agent", "waiting"))
	*clock = clock.Add(9 * time.Minute)
	n.tick()
	*clock = clock.Add(2 * time.Minute)
	n.tick()
	n.tick()
	waitSent(t, sent, "agent finished", "agent needs input", "agent is idle")
}

func TestNotifierSilentWithoutTopicOrEvent(t *testing.T) {
	c := defaultNotifyConfig()
	n, sent, clock := testNotifier(c)
	n.observe("box", state("a", ""))
	n.observe("box", state("a", "waiting"))
	c.Topic = "t"
	c.Waiting = false
	c.Finished = false
	n.config = c
	n.observe("box", state("a", "working"))
	*clock = clock.Add(time.Hour)
	n.observe("box", state("a", ""))
	n.observe("box", state("a", "waiting"))
	time.Sleep(20 * time.Millisecond)
	waitSent(t, sent)
}

func TestNotifierHostsOnlyAnnouncesReturn(t *testing.T) {
	c := defaultNotifyConfig()
	c.Topic = "t"
	c.Hosts = true
	n, sent, _ := testNotifier(c)
	n.hostEvent("box", true)
	n.hostEvent("box", false)
	n.hostEvent("box", true)
	waitSent(t, sent, "box disconnected", "box connected")
}

func TestPostNtfy(t *testing.T) {
	var got *http.Request
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}))
	defer srv.Close()
	c := NotifyConfig{Server: srv.URL, Topic: "portal-751", BaseURL: "https://portal.example"}
	if err := postNtfy(c, notification{Title: "t", Message: "m", Tags: "zzz", Host: "h", Session: "s"}); err != nil {
		t.Fatal(err)
	}
	if got.URL.Path != "/portal-751" || body != "m" || got.Header.Get("Title") != "t" || got.Header.Get("Click") != "https://portal.example/?host=h&session=s" {
		t.Fatalf("unexpected request %s %q %v", got.URL.Path, body, got.Header)
	}
}

func TestNotifyConfigValidation(t *testing.T) {
	for _, c := range []NotifyConfig{
		{Topic: "bad topic", IdleMinutes: 5},
		{Server: "ftp://x", IdleMinutes: 5},
		{IdleMinutes: 0},
	} {
		if err := c.normalize(); err == nil {
			t.Errorf("accepted %+v", c)
		}
	}
	c := NotifyConfig{Topic: " portal-751 ", IdleMinutes: 5}
	if err := c.normalize(); err != nil || c.Server != defaultNtfyServer || c.Topic != "portal-751" {
		t.Fatalf("normalize = %v %+v", err, c)
	}
}
