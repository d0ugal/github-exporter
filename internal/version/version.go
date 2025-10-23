package version

import (
	"fmt"
	"runtime"
)

var (
	Version   string
	Commit    string
	BuildDate string
)

func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
	}
}

type Info struct {
	Version   string
	Commit    string
	BuildDate string
	GoVersion string
}

func (i Info) String() string {
	return fmt.Sprintf("Version: %s, Commit: %s, Build Date: %s, Go Version: %s",
		i.Version, i.Commit, i.BuildDate, i.GoVersion)
}
