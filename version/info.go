package version

import (
	"bytes"
	"runtime"
	"strings"
	"text/template"
)

var (
	// Version is the version number of promutil.
	Version string

	// Revision is the git revision that promutil was built from.
	Revision string

	// Branch is the git branch that promutil was built from.
	Branch string

	// BuildUser is the user that built promutil.
	BuildUser string

	// BuildHost is the host that built promutil.
	BuildHost string

	// BuildDate is the date that promutil was built.
	BuildDate string
	goVersion = runtime.Version()
)

type info struct {
	Version   string
	Revision  string
	Branch    string
	BuildUser string
	BuildHost string
	BuildDate string
	GoVersion string
}

var versionInfoTmpl = `
promutil, version {{.version}} (branch: {{.branch}}, revision: {{.revision}})
  build user:       {{.buildUser}}@{{.buildHost}}
  build date:       {{.buildDate}}
  go version:       {{.goVersion}}
`

// Print formats the version info as a string.
func Print() string {
	m := map[string]string{
		"version":   Version,
		"revision":  Revision,
		"branch":    Branch,
		"buildUser": BuildUser,
		"buildHost": BuildHost,
		"buildDate": BuildDate,
		"goVersion": goVersion,
	}
	t := template.Must(template.New("version").Parse(versionInfoTmpl))

	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "version", m); err != nil {
		panic(err)
	}
	return strings.TrimSpace(buf.String())
}

// NewInfo creates a new version info object.
func NewInfo() info {
	return info{
		Version:   Version,
		Revision:  Revision,
		Branch:    Branch,
		BuildUser: BuildUser,
		BuildHost: BuildHost,
		BuildDate: BuildDate,
		GoVersion: goVersion,
	}
}
