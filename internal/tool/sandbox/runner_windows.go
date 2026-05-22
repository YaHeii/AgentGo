//go:build windows

package sandbox

import (
	"context"
	"errors"
)

// TODO:
// - Implement Job Object with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE.
// - Implement Restricted Token and Low Integrity process startup.
// - Handle workspace ACL / integrity level changes without leaking host state.

type unsupportedRunner struct{}

func NewRunner() Runner {
	return unsupportedRunner{}
}

func (unsupportedRunner) Run(context.Context, Spec) (Result, error) {
	return Result{}, errors.New("sandbox: windows runner is not implemented")
}
