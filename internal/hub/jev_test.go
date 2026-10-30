package hub

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sileod/portal/internal/protocol"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestJevDecisionRequestAndResponse(t *testing.T) {
	j := newJevClassifier(func() bool { return true }, func(jevJob, jevResult) {})
	j.apiKey = "test-key"
	j.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != jevEndpoint || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected request %s %q", r.URL, r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		text := string(body)
		for _, want := range []string{jevModel, "recent_terminal_output", "terminal_status", "inflight", "done", "input"} {
			if !strings.Contains(text, want) {
				t.Fatalf("request missing %q: %s", want, text)
			}
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"model":"typesafe/jev-1.13-20260917","answers":{"terminal_status":{"type":"choice","choice":"input","probabilities":{"inflight":0.01,"done":0.02,"input":0.97},"confidence":0.95}}}`)),
			Header:     make(http.Header),
		}, nil
	})}
	got, err := j.decide(context.Background(), "Approve this command? [y/N]")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "input" || got.Confidence != 0.95 || got.Model != "typesafe/jev-1.13-20260917" {
		t.Fatalf("decision = %+v", got)
	}
}

func TestJevRejectsUnknownChoice(t *testing.T) {
	j := newJevClassifier(func() bool { return true }, func(jevJob, jevResult) {})
	j.apiKey = "test-key"
	j.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(strings.NewReader(`{"model":"m","answers":{"terminal_status":{"type":"choice","choice":"maybe","probabilities":{"inflight":0,"done":0,"input":1},"confidence":1}}}`)), Header: make(http.Header)}, nil
	})}
	if _, err := j.decide(context.Background(), "text"); err == nil || !strings.Contains(err.Error(), "unknown choice") {
		t.Fatalf("error = %v", err)
	}
}

func TestApplyAgentSnapshotKeepsOnlyFreshJevDecision(t *testing.T) {
	a := &agentConn{
		sessionInfos:   []protocol.Session{{Session: "s", JevState: "done", JevConfidence: .9}},
		sessionOutputs: map[string]string{"s": "old"},
	}
	applyAgentSnapshot(a, protocol.Message{SessionInfos: []protocol.Session{{Session: "s"}}, SessionOutputs: map[string]string{"s": "old"}})
	if a.sessionInfos[0].JevState != "done" {
		t.Fatalf("fresh decision was lost: %+v", a.sessionInfos[0])
	}
	applyAgentSnapshot(a, protocol.Message{SessionInfos: []protocol.Session{{Session: "s"}}, SessionOutputs: map[string]string{"s": "new"}})
	if a.sessionInfos[0].JevState != "" {
		t.Fatalf("stale decision was retained: %+v", a.sessionInfos[0])
	}
}

func TestJevPrimeAndDebounceAvoidEagerCalls(t *testing.T) {
	j := newJevClassifier(func() bool { return true }, func(jevJob, jevResult) {})
	j.apiKey = "test-key"
	j.prime("host", map[string]string{"session": "initial output"})
	if len(j.queue) != 0 {
		t.Fatal("priming an existing session queued a paid decision")
	}
	j.observe("host", map[string]string{"session": "initial output"})
	if len(j.queue) != 0 {
		t.Fatal("identical output queued a paid decision")
	}
	j.observe("host", map[string]string{"session": "changed output"})
	j.mu.Lock()
	timerSet := j.entries["host\x00session"].timer != nil
	j.entries["host\x00session"].timer.Stop()
	j.mu.Unlock()
	if !timerSet || len(j.queue) != 0 {
		t.Fatal("changed output was not debounced")
	}
}
