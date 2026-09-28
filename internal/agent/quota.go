package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/sileod/portal/internal/protocol"
)

const quotaSampleInterval = 5 * time.Minute

var quotaSampleCache struct {
	sync.Mutex
	at     time.Time
	quotas map[string]protocol.QuotaResult
}

func hlinkPath() string {
	if path := os.Getenv("PORTAL_HLINK"); path != "" {
		return path
	}
	if path, err := exec.LookPath("hlink"); err == nil {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, ".local", "bin", "hlink")
	if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
		return path
	}
	return ""
}

// All hub connections in this process share one quota query. Harness Link
// reads the host's existing harness logins and returns only quota summaries.
func sampleQuotas() map[string]protocol.QuotaResult {
	quotaSampleCache.Lock()
	defer quotaSampleCache.Unlock()
	if time.Since(quotaSampleCache.at) < quotaSampleInterval {
		return quotaSampleCache.quotas
	}
	quotaSampleCache.at = time.Now()
	quotaSampleCache.quotas = nil
	path := hlinkPath()
	if path == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	output, _ := exec.CommandContext(ctx, path, "quota", "--json").Output()
	var quotas map[string]protocol.QuotaResult
	if json.Unmarshal(output, &quotas) != nil || len(quotas) == 0 {
		return nil
	}
	quotaSampleCache.quotas = quotas
	return quotas
}

func (c *connection) publishQuotas(done <-chan struct{}) {
	ticker := time.NewTicker(quotaSampleInterval)
	defer ticker.Stop()
	for {
		if err := c.write(protocol.Message{Type: "quotas", Quotas: sampleQuotas()}); err != nil {
			return
		}
		select {
		case <-done:
			return
		case <-ticker.C:
		}
	}
}
