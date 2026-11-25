//go:build !darwin && !linux

package permissionhook

import "os"

func loggingSupported() bool { return false }

func appendPrivateRecord(_ string, _ []byte) error {
	return ErrLogUnsupported
}

func openPrivateLog(_ string) (*os.File, error) {
	return nil, ErrLogUnsupported
}
