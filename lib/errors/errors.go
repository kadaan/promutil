package errors

import (
	"fmt"
	"github.com/hashicorp/go-multierror"
	"github.com/kadaan/tracerr"
	//errors2 "github.com/prometheus/prometheus/tsdb/errors"
)

var (
	customTracerr = tracerr.NewTracerr(tracerr.DefaultFrameCapacity, tracerr.DefaultFrameSkipCount+1)
)

func New(message string, a ...any) tracerr.Error {
	return customTracerr.New(fmt.Sprintf(message, a...))
}

func NewMulti(errs []error, message string, a ...any) tracerr.Error {
	var multiError *multierror.Error
	for _, err := range errs {
		multiError = multierror.Append(err)
	}
	return customTracerr.Wrap(fmt.Errorf("%s: %w", fmt.Sprintf(message, a...), multiError))
}

func Errorf(format string, a ...any) tracerr.Error {
	return customTracerr.Wrap(fmt.Errorf(format, a))
}

func Wrap(err error, message string, a ...any) tracerr.Error {
	if err == nil {
		return nil
	}
	return customTracerr.Wrap(fmt.Errorf("%s: %w", fmt.Sprintf(message, a...), err))
}

func ToString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
