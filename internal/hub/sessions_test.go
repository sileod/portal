package hub

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sileod/portal/internal/auth"
)

func login(t *testing.T, s *Server, password string) *http.Cookie {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "http://portal/login", strings.NewReader(url.Values{"password": {password}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.RemoteAddr = "192.0.2.9:1"
	w := httptest.NewRecorder()
	s.handleLogin(w, r)
	for _, c := range w.Result().Cookies() {
		if c.Name == "portal_session" && c.Value != "" {
			return c
		}
	}
	t.Fatalf("login failed: %d", w.Code)
	return nil
}

func loggedIn(s *Server, c *http.Cookie) bool {
	r := httptest.NewRequest(http.MethodGet, "http://portal/", nil)
	r.AddCookie(c)
	return s.browserOK(r)
}

func TestBrowserLoginsSurviveHubRestart(t *testing.T) {
	hash, err := auth.HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "hub-sessions.json")

	first := New(hash, "token")
	if err := first.PersistSessions(path); err != nil {
		t.Fatal(err)
	}
	kept, dropped := login(t, first, "pw"), login(t, first, "pw")

	logout := httptest.NewRequest(http.MethodGet, "http://portal/logout", nil)
	logout.AddCookie(dropped)
	first.handleLogout(httptest.NewRecorder(), logout)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), kept.Value) {
		t.Fatal("sessions file must not contain usable cookie values")
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0600 {
		t.Fatalf("sessions file mode = %v, want 0600", info.Mode().Perm())
	}

	restarted := New(hash, "token")
	if err := restarted.PersistSessions(path); err != nil {
		t.Fatal(err)
	}
	if !loggedIn(restarted, kept) {
		t.Fatal("login did not survive a hub restart")
	}
	if loggedIn(restarted, dropped) {
		t.Fatal("logged-out browser came back after restart")
	}

	newHash, _ := auth.HashPassword("new password")
	changed := New(newHash, "token")
	if err := changed.PersistSessions(path); err != nil {
		t.Fatal(err)
	}
	if loggedIn(changed, kept) {
		t.Fatal("changing the password must log browsers out")
	}
}
