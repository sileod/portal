package agent

import "testing"

func TestParseGPUCSV(t *testing.T) {
	gpus := parseGPUCSV("0, NVIDIA A100, 42, 1200, 40960\n1, \"GPU, test\", 0, 0, 8192\n")
	if len(gpus) != 2 || gpus[0].Utilization != 42 || gpus[0].MemoryTotal != 40960 || gpus[1].Name != "GPU, test" {
		t.Fatalf("unexpected GPUs: %+v", gpus)
	}
	if got := parseGPUCSV("0, GPU, N/A, 0, 8192\n"); len(got) != 0 {
		t.Fatalf("invalid utilization should be omitted: %+v", got)
	}
}
