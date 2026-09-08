package buildinfo

import (
	"runtime/debug"
	"strings"
)

// Version and Revision are set for release builds. Current falls back to Go's
// embedded VCS revision so local builds still identify their source.
var (
	Version  = ""
	Revision = ""
)

func Current() string {
	version := strings.TrimSpace(Version)
	revision := shortRevision(Revision)
	if version != "" {
		if revision != "" {
			return version + " (" + revision + ")"
		}
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			if revision := shortRevision(setting.Value); revision != "" {
				return "dev (" + revision + ")"
			}
		}
	}
	return "dev"
}

func shortRevision(revision string) string {
	revision = strings.TrimSpace(revision)
	if len(revision) > 7 {
		return revision[:7]
	}
	return revision
}
