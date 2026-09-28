package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSampleQuotasAcceptsJSONWhenSomeProvidersFail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hlink")
	script := "#!/bin/sh\nprintf '%s\\n' '{\"codex\":{\"ok\":true,\"windows\":[{\"name\":\"5h\",\"remaining\":78}]},\"opencode\":{\"ok\":false,\"error\":\"not logged in\"}}'\nexit 1\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	quotaSampleCache.Lock()
	quotaSampleCache.at = time.Time{}
	quotaSampleCache.quotas = nil
	quotaSampleCache.Unlock()
	t.Cleanup(func() {
		quotaSampleCache.Lock()
		quotaSampleCache.at = time.Time{}
		quotaSampleCache.quotas = nil
		quotaSampleCache.Unlock()
	})

	quotas := sampleQuotas()
	if !quotas["codex"].OK || len(quotas["codex"].Windows) != 1 || quotas["codex"].Windows[0].Remaining != 78 {
		t.Fatalf("unexpected Codex quota: %+v", quotas["codex"])
	}
	if quotas["opencode"].OK || quotas["opencode"].Error != "not logged in" {
		t.Fatalf("unexpected OpenCode quota: %+v", quotas["opencode"])
	}
}
