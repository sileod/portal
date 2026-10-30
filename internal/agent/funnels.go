package agent

import (
	"context"
	"encoding/json"
	"net"
	"net/url"
	"os/exec"
	"sort"
	"sync"
	"time"

	"github.com/sileod/portal/internal/protocol"
)

type funnelConfig struct {
	Web         map[string]struct{ Handlers map[string]json.RawMessage }
	AllowFunnel map[string]bool
	Foreground  map[string]funnelConfig
}

// Discover only public HTTPS routes, without exposing local proxy targets.
func parseFunnels(data []byte) []protocol.Funnel {
	var config funnelConfig
	if json.Unmarshal(data, &config) != nil {
		return nil
	}
	routes := map[string]bool{}
	var collect func(funnelConfig)
	collect = func(c funnelConfig) {
		for address, web := range c.Web {
			if !c.AllowFunnel[address] {
				continue
			}
			host, port, err := net.SplitHostPort(address)
			if err != nil || host == "" {
				continue
			}
			authority := address
			if port == "443" {
				authority = host
			}
			for path := range web.Handlers {
				if len(path) == 0 || path[0] != '/' {
					continue
				}
				u := url.URL{Scheme: "https", Host: authority, Path: path}
				routes[u.String()] = true
			}
		}
		for _, foreground := range c.Foreground {
			collect(foreground)
		}
	}
	collect(config)
	result := make([]protocol.Funnel, 0, len(routes))
	for address := range routes {
		result = append(result, protocol.Funnel{URL: address})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].URL < result[j].URL })
	return result
}

var funnelCache struct {
	sync.Mutex
	at     time.Time
	routes []protocol.Funnel
}

func sampleFunnels() []protocol.Funnel {
	funnelCache.Lock()
	defer funnelCache.Unlock()
	if time.Since(funnelCache.at) < time.Minute {
		return funnelCache.routes
	}
	funnelCache.at = time.Now()
	funnelCache.routes = nil
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "tailscale", "funnel", "status", "--json").Output()
	if err == nil {
		funnelCache.routes = parseFunnels(output)
	}
	return funnelCache.routes
}

func (c *connection) publishFunnels(done <-chan struct{}) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := c.write(protocol.Message{Type: "funnels", Funnels: sampleFunnels()}); err != nil {
			return
		}
		select {
		case <-done:
			return
		case <-ticker.C:
		}
	}
}
