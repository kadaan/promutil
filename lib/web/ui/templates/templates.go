//go:build dev
// +build dev

package templates

import (
	"github.com/kadaan/promutil/lib/web/ui"
	"net/http"
	"os"
	"strings"

	"github.com/shurcooL/httpfs/filter"
)

var Templates = filter.Keep(
	http.Dir(ui.ImportPathToDir("./lib/web/ui/templates/assets")),
	func(path string, fi os.FileInfo) bool {
		return fi.IsDir() || strings.HasSuffix(path, ".html")
	},
)
