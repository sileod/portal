package hub

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSecurityHeadersAllowXtermCDN(t *testing.T) {
	w := httptest.NewRecorder()
	securityHeaders(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(w, httptest.NewRequest("GET", "http://portal/", nil))
	csp := w.Header().Get("Content-Security-Policy")
	if strings.Count(csp, "https://cdnjs.cloudflare.com") != 2 {
		t.Fatalf("CSP must allow the xterm CDN for scripts and styles: %q", csp)
	}
}

func TestClientIPUsesProxyAppendedAddress(t *testing.T) {
	r := httptest.NewRequest("GET", "http://portal/", nil)
	r.RemoteAddr = "127.0.0.1:12345"
	r.Header.Set("X-Forwarded-For", "198.51.100.7, 203.0.113.9")
	if got := clientIP(r); got != "203.0.113.9" {
		t.Fatalf("clientIP = %q, want proxy-appended address", got)
	}
}

func TestClientIPIgnoresForwardingFromUntrustedPeer(t *testing.T) {
	r := httptest.NewRequest("GET", "http://portal/", nil)
	r.RemoteAddr = "192.0.2.20:12345"
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := clientIP(r); got != "192.0.2.20" {
		t.Fatalf("clientIP = %q, want direct peer", got)
	}
}

func TestAuthBackoff(t *testing.T) {
	s := New("unused", "token")
	if got := s.authFailure("192.0.2.1"); got != time.Second {
		t.Fatalf("first delay = %v, want 1s", got)
	}
	if got := s.authWait("192.0.2.1"); got <= 0 || got > time.Second {
		t.Fatalf("authWait = %v after failure", got)
	}
	s.authSuccess("192.0.2.1")
	if got := s.authWait("192.0.2.1"); got != 0 {
		t.Fatalf("authWait after success = %v, want 0", got)
	}
}

func TestSafeBrowserNext(t *testing.T) {
	want := "/?host=gpu-box&session=work"
	if got := safeBrowserNext(want); got != want {
		t.Fatalf("safeBrowserNext = %q, want %q", got, want)
	}
	for _, unsafe := range []string{"https://evil.example/", "//evil.example/", "/other", ""} {
		if got := safeBrowserNext(unsafe); got != "/" {
			t.Fatalf("safeBrowserNext(%q) = %q, want /", unsafe, got)
		}
	}
}

func TestIndexIsNotCachedAcrossPortalUpdates(t *testing.T) {
	s := New("unused", "token")
	s.sessions["browser"] = time.Now().Add(time.Hour)
	r := httptest.NewRequest(http.MethodGet, "http://portal/", nil)
	r.AddCookie(&http.Cookie{Name: "portal_session", Value: "browser"})
	w := httptest.NewRecorder()

	s.handleIndex(w, r)

	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}
