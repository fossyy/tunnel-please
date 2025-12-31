package version

import "fmt"

var (
	Version   = "dev"
	BuildDate = "unknown"
	Commit    = "unknown"
)

func GetVersion() string {
	return fmt.Sprintf("tunnel_pls %s (commit: %s, built: %s)", Version, Commit, BuildDate)
}

func GetShortVersion() string {
	return Version
}
