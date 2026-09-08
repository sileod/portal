package buildinfo

import (
	"runtime/debug"
	"strings"
)

// Version can be set at build time. Current falls back to Go's embedded VCS
// revision so local builds still identify the source they came from.
var Version = ""

func Current() string {
	if version := short(Version); version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return short(setting.Value)
		}
	}
	return "dev"
}

func short(version string) string {
	version = strings.TrimSpace(version)
	if len(version) > 12 {
		return version[:12]
	}
	return version
}
