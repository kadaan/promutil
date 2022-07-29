//go:build ignore
// +build ignore

package main

import (
	"log"
	"path"

	"github.com/kadaan/promutil/lib/errors"
	"github.com/kadaan/promutil/lib/web/ui"
	"github.com/kadaan/promutil/lib/web/ui/static"
	"github.com/shurcooL/vfsgen"
)

func main() {
	rootDir := ui.PathToProjectDir("./lib/web/ui/static")
	if err := vfsgen.Generate(static.Static, vfsgen.Options{
		Filename:     path.Join(rootDir, "static_vfsdata.go"),
		PackageName:  "static",
		BuildTags:    "!dev",
		VariableName: "Static",
	}); err != nil {
		log.Fatalln(errors.Wrap(err, "failed to generate Static vFS"))
	}
}
