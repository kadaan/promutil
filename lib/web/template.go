package web

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/kadaan/promutil/lib/web/ui/templates"
	"github.com/kadaan/promutil/version"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/template"
	"io"
	"io/ioutil"
	"net/http"
	textTemplate "text/template"
	"time"
)

type Options struct {
	Version     version.Info
	NavBarLinks []NavBarLink
}

type TemplateExecutor interface {
	ExecuteTemplate(requestContext *gin.Context, name string)
}

func NewTemplateExecutor(options *Options) TemplateExecutor {
	return &templateExecutor{options: options}
}

type templateExecutor struct {
	options *Options
}

func (e *templateExecutor) ExecuteTemplate(requestContext *gin.Context, name string) {
	text, err := e.getTemplate(name)
	if err != nil {
		_ = requestContext.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	tmpl := template.NewTemplateExpander(
		requestContext.Request.Context(),
		text,
		name,
		nil,
		model.TimeFromUnix(time.Now().Unix()),
		nil,
		nil,
		nil,
	)

	tmpl.Funcs(e.tmplFuncs(e.options))

	result, err := tmpl.ExpandHTML(nil)
	if err != nil {
		_ = requestContext.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	_, _ = io.WriteString(requestContext.Writer, result)
}

func (e *templateExecutor) getTemplate(name string) (string, error) {
	var tmpl string

	appendFunc := func(name string) error {
		f, err := templates.Templates.Open(name)
		if err != nil {
			return err
		}
		defer func(f http.File) {
			_ = f.Close()
		}(f)
		b, err := ioutil.ReadAll(f)
		if err != nil {
			return err
		}
		tmpl += string(b)
		return nil
	}

	err := appendFunc("_base.html")
	if err != nil {
		return "", fmt.Errorf("error reading base template: %s", err)
	}
	err = appendFunc(name)
	if err != nil {
		return "", fmt.Errorf("error reading page template %s: %s", name, err)
	}

	return tmpl, nil
}

func (e *templateExecutor) tmplFuncs(opts *Options) textTemplate.FuncMap {
	return textTemplate.FuncMap{
		"pathPrefix":   func() string { return "" },
		"buildVersion": func() string { return fmt.Sprintf("%s_%s", opts.Version.Version, opts.Version.Revision) },
		"navBarLinks":  func() []NavBarLink { return opts.NavBarLinks },
	}
}
