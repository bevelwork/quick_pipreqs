package version

import "fmt"

const (
	// Update these via make targets
	Major     = 1
	Minor     = 5
	PatchDate = "20250916" // YYYYMMDD
)

var Full = fmt.Sprintf("%d.%d.%s", Major, Minor, PatchDate)
