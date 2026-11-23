//go:build darwin || linux || dragonfly || freebsd || netbsd || openbsd

package permissionhook

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func loggingDiagnostic() {
	fd, err := unix.FcntlInt(os.Stderr.Fd(), unix.F_DUPFD_CLOEXEC, 3)
	if err != nil {
		return
	}
	file := os.NewFile(uintptr(fd), "hook-diagnostic")
	defer file.Close()
	_, _ = fmt.Fprintln(file, "permission hook: unable to record invocation")
}
