//go:build linux

package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const sandboxWorkspaceDir = "/workspace"

type bwrapRunner struct{}

func NewRunner() Runner {
	return bwrapRunner{}
}

func (bwrapRunner) Run(ctx context.Context, spec Spec) (Result, error) {
	args, err := buildBwrapArgs(spec)
	if err != nil {
		return Result{}, err
	}

	cmd := exec.CommandContext(ctx, "bwrap", args...)
	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), envList(spec.Env)...)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	result := Result{
		ExitCode: 0,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
	if err == nil {
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return result, err
}

func buildBwrapArgs(spec Spec) ([]string, error) {
	if strings.TrimSpace(spec.Executable) == "" {
		return nil, fmt.Errorf("sandbox executable is required")
	}

	workspaceDir, err := cleanAbsPath(spec.WorkspaceDir)
	if err != nil {
		return nil, fmt.Errorf("sandbox workspace_dir: %w", err)
	}

	args := []string{
		"--bind", workspaceDir, sandboxWorkspaceDir,
		"--chdir", sandboxWorkspaceDir,
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
	}

	for _, dir := range defaultReadOnlyDirs() {
		args = append(args, "--ro-bind", dir, dir)
	}
	for _, dir := range spec.ReadOnlyDirs {
		cleaned, err := cleanAbsPath(dir)
		if err != nil {
			return nil, fmt.Errorf("sandbox readonly dir: %w", err)
		}
		args = append(args, "--ro-bind", cleaned, cleaned)
	}

	args = append(args, strings.TrimSpace(spec.Executable))
	args = append(args, spec.Args...)
	return args, nil
}

func defaultReadOnlyDirs() []string {
	candidates := []string{
		"/usr",
		"/bin",
		"/lib",
		"/lib64",
		"/etc/ssl",
		"/etc/ca-certificates",
	}
	dirs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			dirs = append(dirs, candidate)
		}
	}
	return dirs
}

func cleanAbsPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%q must be absolute", path)
	}
	return filepath.Clean(path), nil
}

func envList(env map[string]string) []string {
	items := make([]string, 0, len(env))
	for key, value := range env {
		items = append(items, key+"="+value)
	}
	return items
}
