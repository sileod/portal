package agent

import (
	"context"
	"encoding/csv"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sileod/portal/internal/protocol"
)

const gpuSampleInterval = 30 * time.Second

var gpuSampleCache struct {
	sync.Mutex
	at   time.Time
	gpus []protocol.GPU
}

// sampleGPUs is shared by all hub connections in this process, so a host
// connected to several hubs still runs nvidia-smi at most once per interval.
func sampleGPUs() []protocol.GPU {
	gpuSampleCache.Lock()
	defer gpuSampleCache.Unlock()
	if time.Since(gpuSampleCache.at) < gpuSampleInterval {
		return gpuSampleCache.gpus
	}
	gpuSampleCache.at = time.Now()
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		gpuSampleCache.gpus = nil
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--query-gpu=index,name,utilization.gpu,memory.used,memory.total", "--format=csv,noheader,nounits").Output()
	if err != nil {
		gpuSampleCache.gpus = nil
		return nil
	}
	gpuSampleCache.gpus = parseGPUCSV(string(out))
	return gpuSampleCache.gpus
}

func parseGPUCSV(data string) []protocol.GPU {
	r := csv.NewReader(strings.NewReader(data))
	r.TrimLeadingSpace = true
	var gpus []protocol.GPU
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(row) != 5 {
			return nil
		}
		index, e1 := strconv.Atoi(strings.TrimSpace(row[0]))
		util, e2 := strconv.Atoi(strings.TrimSpace(row[2]))
		used, e3 := strconv.Atoi(strings.TrimSpace(row[3]))
		total, e4 := strconv.Atoi(strings.TrimSpace(row[4]))
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil || util < 0 || util > 100 || used < 0 || total < 0 {
			continue
		}
		gpus = append(gpus, protocol.GPU{Index: index, Name: strings.TrimSpace(row[1]), Utilization: util, MemoryUsed: used, MemoryTotal: total})
	}
	return gpus
}

func (c *connection) publishGPUs(done <-chan struct{}) {
	ticker := time.NewTicker(gpuSampleInterval)
	defer ticker.Stop()
	for {
		if err := c.write(protocol.Message{Type: "gpus", GPUs: sampleGPUs()}); err != nil {
			return
		}
		select {
		case <-done:
			return
		case <-ticker.C:
		}
	}
}
