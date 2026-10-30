package agent

import "testing"

func TestParseFunnels(t *testing.T) {
	data := []byte(`{"Web":{"apps.example:443":{"Handlers":{"/":{},"/opencode":{}}},"private.example:443":{"Handlers":{"/":{}}},"off.example:443":{"Handlers":{"/":{}}}},"AllowFunnel":{"apps.example:443":true,"off.example:443":false},"Foreground":{"1":{"Web":{"apps.example:8443":{"Handlers":{"/gui":{}}}},"AllowFunnel":{"apps.example:8443":true}}}}`)
	routes := parseFunnels(data)
	want := []string{"https://apps.example/", "https://apps.example/opencode", "https://apps.example:8443/gui"}
	if len(routes) != len(want) {
		t.Fatalf("unexpected routes: %+v", routes)
	}
	for i, route := range routes {
		if route.URL != want[i] {
			t.Fatalf("route %d: %s, want %s", i, route.URL, want[i])
		}
	}
	if got := parseFunnels([]byte(`invalid`)); len(got) != 0 {
		t.Fatalf("invalid JSON: %+v", got)
	}
}
