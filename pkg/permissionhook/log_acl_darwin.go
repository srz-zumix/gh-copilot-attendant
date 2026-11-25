//go:build darwin && cgo

package permissionhook

/*
#include <errno.h>
#include <sys/acl.h>

static int has_extended_acl(int fd) {
	errno = 0;
	acl_t acl = acl_get_fd_np(fd, ACL_TYPE_EXTENDED);
	if (acl == NULL) {
		return errno == ENOENT ? 0 : -1;
	}
	acl_free(acl);
	return 1;
}
*/
import "C"

import "errors"

func loggingSupported() bool { return true }

func checkPrivateACL(fd int) error {
	if C.has_extended_acl(C.int(fd)) != 0 {
		return errors.New("unsafe or unreadable log ACL")
	}
	return nil
}
