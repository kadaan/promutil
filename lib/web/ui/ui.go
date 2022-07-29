package ui

import (
	"github.com/kadaan/promutil/lib/errors"
	"go/build"
	"log"
	"os"
	"path"
)

func PathToProjectDir(importPath string) string {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalln(errors.Wrap(err, "failed to determine working directory"))
	}
	importRoot := wd
	importDir := path.Join(importRoot, importPath)
	for {
		if _, err := os.Stat(importDir); err == nil {
			break
		}
		if importRoot == "/" {
			log.Fatalln(errors.New("failed to find import path '%s' under '%s' or any parents", importPath, wd))
		}
		importRoot = path.Dir(importRoot)
		importDir = path.Join(importRoot, importPath)
	}
	return importDir
}

func ImportPathToDir(importPath string) string {
	p, err := build.ImportDir(PathToProjectDir(importPath), build.FindOnly)
	if err != nil {
		log.Fatalln(err)
	}
	return p.Dir
}
