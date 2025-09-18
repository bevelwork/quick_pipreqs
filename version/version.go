package version

import "fmt"

const (
	// Update these via make targets
	Major     = 1
	Minor     = 7
	PatchDate = "20250918" // YYYYMMDD
)

var Full = fmt.Sprintf("%d.%d.%s", Major, Minor, PatchDate)
