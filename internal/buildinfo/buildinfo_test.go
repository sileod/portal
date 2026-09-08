package buildinfo

import "testing"

func TestCurrentReleaseVersion(t *testing.T) {
	oldVersion, oldRevision := Version, Revision
	Version, Revision = "v70", "4300feae1ae2ff128c8f62f5fa6def6ed5666e0f"
	t.Cleanup(func() { Version, Revision = oldVersion, oldRevision })

	if got, want := Current(), "v70 (4300fea)"; got != want {
		t.Fatalf("Current() = %q, want %q", got, want)
	}
}
