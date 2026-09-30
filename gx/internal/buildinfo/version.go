package buildinfo

import "runtime/debug"

const modulePath = "github.com/lanechi/gonex/gx"

// Version reports the version embedded in the gx module build information.
// Source builds return (devel) when Go has no module version to report.
func Version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Path != modulePath || info.Main.Version == "" {
		return "(devel)"
	}
	return info.Main.Version
}
