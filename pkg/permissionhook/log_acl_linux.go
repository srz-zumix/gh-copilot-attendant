//go:build linux

package permissionhook

func loggingSupported() bool { return true }

// Linux ACL permissions for named users and groups are bounded by the mode's group mask.
func checkPrivateACL(_ int) error { return nil }
