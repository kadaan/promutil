//go:build !dev
// +build !dev

package version

var (
	// Version is the version number of the program.
	Version string

	// Revision is the git revision that program was built from.
	Revision string

	// Branch is the git branch that program was built from.
	Branch string

	// BuildUser is the user that built program.
	BuildUser string

	// BuildHost is the host that built program.
	BuildHost string

	// BuildDate is the date that program was built.
	BuildDate string
)
