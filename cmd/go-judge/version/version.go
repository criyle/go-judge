package version

import (
	"embed"
	"io"
	"runtime/debug"
	"strings"
)

//go:embed version.*
var versions embed.FS

var Version string = "unable to get version"

func init() {
	f, err := versions.Open("version.txt")
	if err != nil {
		// go generate was not run, assuming installed by go install
		// get version information from debug
		inf, ok := debug.ReadBuildInfo()
		if !ok {
			return
		}
		Version = inf.Main.Version
		return
	}
	s, err := io.ReadAll(f)
	if err != nil {
		return
	}
	Version = strings.TrimSpace(string(s))
}
