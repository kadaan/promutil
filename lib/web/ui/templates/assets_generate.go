//go:build ignore
// +build ignore

package main

import (
	"log"
	"path"

	"github.com/kadaan/promutil/lib/errors"
	"github.com/kadaan/promutil/lib/web/ui"
	"github.com/kadaan/promutil/lib/web/ui/templates"
	"github.com/shurcooL/vfsgen"
)

func main() {
	rootDir := ui.PathToProjectDir("./lib/web/ui/templates")
	if err := vfsgen.Generate(templates.Templates, vfsgen.Options{
		Filename:     path.Join(rootDir, "templates_vfsdata.go"),
		PackageName:  "templates",
		BuildTags:    "!dev",
		VariableName: "Templates",
	}); err != nil {
		log.Fatalln(errors.Wrap(err, "failed to generate Templates vFS"))
	}
}
