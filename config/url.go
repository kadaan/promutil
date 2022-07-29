package config

import (
	"fmt"
	"github.com/kadaan/promutil/lib/errors"
	"net/url"
	"strings"
)

type urlValue struct {
	value **url.URL
}

func NewUrlValue(p **url.URL, val *url.URL) *urlValue {
	uv := new(urlValue)
	uv.value = p
	*uv.value = val
	return uv
}

// String is used both by fmt.Print and by Cobra in help text
func (e *urlValue) String() string {
	return fmt.Sprintf("\"%s\"", (*e.value).String())
}

// Set must have pointer receiver, so it doesn't change the value of a copy
func (e *urlValue) Set(v string) error {
	u, err := url.Parse(v)
	if err != nil {
		return errors.Wrap(err, "failed to parse url")
	}
	if strings.ToUpper(u.Scheme) != "HTTP" && strings.ToUpper(u.Scheme) != "HTTPS" {
		return errors.New("url must be have scheme set to HTTP or HTTPS")
	}
	*e.value = u
	return nil
}

// Type is only used in help text
func (e *urlValue) Type() string {
	return "url"
}

func MustParseUrl(value string) *url.URL {
	u, err := url.Parse(fmt.Sprintf(value))
	if err != nil {
		panic(errors.Wrap(err, "failed to parse url: %s", value))
	}
	return u
}
