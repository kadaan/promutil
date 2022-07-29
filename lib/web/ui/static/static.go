//go:build dev
// +build dev

package static

import (
	"github.com/kadaan/promutil/lib/web/ui"
	"net/http"
	"os"
	"strings"

	"github.com/shurcooL/httpfs/filter"
)

var Static = filter.Keep(
	http.Dir(ui.ImportPathToDir("./lib/web/ui/static/assets")),
	func(path string, fi os.FileInfo) bool {
		return fi.IsDir() ||
			(!strings.HasSuffix(path, "map.js") &&
				!strings.HasSuffix(path, "/bootstrap.js") &&
				!strings.HasSuffix(path, "/bootstrap-theme.css") &&
				!strings.HasSuffix(path, "/bootstrap.css"))
	},
)
