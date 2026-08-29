package hub

import (
	"net/http/httptest"
	"testing"
	"time"
)

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
