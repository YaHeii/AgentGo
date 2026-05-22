//go:build linux

package sandbox

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildBwrapArgs(t *testing.T) {
	t.Parallel()

	args, err := buildBwrapArgs(Spec{
		Executable:   "bash",
		Args:         []string{"-lc", "pwd"},
		WorkspaceDir: "/tmp/ws",
	})

	require.NoError(t, err)
	require.Contains(t, args, "--bind")
	require.Contains(t, args, "/tmp/ws")
	require.Contains(t, args, "/workspace")
	require.Contains(t, args, "--chdir")
	require.False(t, slices.Contains(args, "--unshare-net"))
	require.False(t, slices.Contains(args, "--unshare-all"))
	require.Equal(t, "bash", args[len(args)-3])
	require.Equal(t, "-lc", args[len(args)-2])
	require.Equal(t, "pwd", args[len(args)-1])
}
