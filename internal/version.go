package version

import (
	"fmt"
	"strings"
)

const (
	Major      = 1
	Minor      = 0
	Patch      = 0
	PreRelease = ""
)

func String() string {
	v := fmt.Sprintf("v%d.%d.%d", Major, Minor, Patch)
	if PreRelease != "" {
		v += "-" + PreRelease

		var metadata string
		if strings.Contains(PreRelease, "devel") && metadata != "" {
			v += "+" + metadata
		}
	}
	return v
}
