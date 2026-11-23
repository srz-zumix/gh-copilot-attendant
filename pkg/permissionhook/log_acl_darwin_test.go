//go:build darwin && cgo

package permissionhook

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHookLogRejectsACL(t *testing.T) {
	for _, target := range []string{"directory", "file"} {
		t.Run(target, func(t *testing.T) {
			t.Setenv("COPILOT_HOME", t.TempDir())
			require.NoError(t, appendHookRecord(HookRecord{Version: 1}))
			path, err := HookLogPath()
			require.NoError(t, err)
			if target == "directory" {
				path = filepath.Dir(path)
			}
			require.NoError(t, exec.Command("/bin/chmod", "+a", "everyone allow read", path).Run())
			require.Error(t, appendHookRecord(HookRecord{Version: 1}))
		})
	}
}
