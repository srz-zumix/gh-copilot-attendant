//go:build darwin && !cgo

package permissionhook

import "errors"

func loggingSupported() bool { return false }

func checkPrivateACL(_ int) error {
	return errors.New("private hook logging requires ACL inspection")
}
