//go:build !linux && !windows

package sandbox

import (
	"context"
	"errors"
)

type unsupportedRunner struct{}

func NewRunner() Runner {
	return unsupportedRunner{}
}

func (unsupportedRunner) Run(context.Context, Spec) (Result, error) {
	return Result{}, errors.New("sandbox: runner is not implemented for this platform")
}
