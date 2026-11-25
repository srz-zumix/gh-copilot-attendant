//go:build !darwin && !linux && !dragonfly && !freebsd && !netbsd && !openbsd

package permissionhook

import (
	"fmt"
	"os"
)

func loggingDiagnostic() {
	_, _ = fmt.Fprintln(os.Stderr, "permission hook: unable to record invocation")
}
