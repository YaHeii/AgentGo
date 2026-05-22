package sandbox

import "context"

type Spec struct {
	Executable   string
	Args         []string
	Env          map[string]string
	WorkspaceDir string
	ReadOnlyDirs []string
}

type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

type Runner interface {
	Run(ctx context.Context, spec Spec) (Result, error)
}
