//go:build darwin || linux

package sessionstats

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/srz-zumix/go-gh-extension/pkg/copilotext"
	"golang.org/x/sys/unix"
)

func TestCollectHookActivityRejectsUnsafeLogPaths(t *testing.T) {
	for _, kind := range []string{"file-symlink", "directory-symlink", "fifo", "directory", "permissions"} {
		t.Run(kind, func(t *testing.T) {
			_, root, path := hookFixture(t)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "target")
			if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "file-symlink":
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "directory-symlink":
				dir := filepath.Dir(path)
				if err := os.Remove(dir); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Dir(target), dir); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := unix.Mkfifo(path, 0o600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			case "permissions":
				if err := os.WriteFile(path, []byte("unsafe"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			result := CollectHookActivity(copilotext.PermissionStatsOptions{Root: root})
			if result.State != "unreadable" || result.Data != nil {
				t.Fatalf("unsafe %s = %+v", kind, result)
			}
			content, err := os.ReadFile(target)
			if err != nil || string(content) != "unchanged" {
				t.Fatalf("target changed: %q, %v", content, err)
			}
		})
	}
}
