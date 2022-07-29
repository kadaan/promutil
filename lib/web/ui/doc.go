package ui

import (
	// The blank import is to make govendor happy.
	_ "github.com/shurcooL/vfsgen"
)

//go:generate go run -tags=dev ./static/assets_generate.go
//go:generate go run -tags=dev ./templates/assets_generate.go
