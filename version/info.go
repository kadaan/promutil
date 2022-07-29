package version

import (
	"bytes"
	"runtime"
	"strings"
	"text/template"
)

const (
	Name = "promutil"
)

var (
	goVersion = runtime.Version()
)

var versionInfoTmpl = `
{{.name}}, version {{.version}} (branch: {{.branch}}, revision: {{.revision}})
  build user:       {{.buildUser}}@{{.buildHost}}
  build date:       {{.buildDate}}
  go version:       {{.goVersion}}
`

// Print formats the version info as a string.
func Print() string {
	m := map[string]string{
		"name":      Name,
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

type Info struct {
	Version   string
	Revision  string
	Branch    string
	BuildUser string
	BuildDate string
	GoVersion string
}

func NewInfo() Info {
	return Info{
		Version:   Version,
		Revision:  Revision,
		Branch:    Branch,
		BuildUser: BuildUser,
		BuildDate: BuildDate,
		GoVersion: goVersion,
	}
}
