//go:build darwin || linux

package permissionhook

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func appendPrivateRecord(path string, data []byte) error {
	dir, name, err := openPrivateLogDirectory(path, true)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(dir) }()
	var fd int
	for attempts := 0; attempts < 3; attempts++ {
		fd, err = unix.Openat(dir, name, unix.O_WRONLY|unix.O_APPEND|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0o600)
		if !errors.Is(err, unix.ENOENT) {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer func() { _ = unix.Close(fd) }()
	if err := validatePrivateLogFile(fd); err != nil {
		return err
	}
	deadline := time.Now().Add(100 * time.Millisecond)
	for {
		err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) || !time.Now().Before(deadline) {
			return err
		}
		time.Sleep(5 * time.Millisecond)
	}
	defer func() { _ = unix.Flock(fd, unix.LOCK_UN) }()
	n, err := unix.Write(fd, data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return fmt.Errorf("write hook log: %w", io.ErrShortWrite)
	}
	return nil
}

func openPrivateLog(path string) (*os.File, error) {
	dir, name, err := openPrivateLogDirectory(path, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = unix.Close(dir) }()
	fd, err := unix.Openat(dir, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	if err := validatePrivateLogFile(fd); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func openPrivateLogDirectory(path string, create bool) (int, string, error) {
	if !loggingSupported() {
		return -1, "", ErrLogUnsupported
	}
	profile := filepath.Dir(filepath.Dir(filepath.Dir(path)))
	resolved, err := filepath.EvalSymlinks(profile)
	if errors.Is(err, os.ErrNotExist) && create {
		parent, parentErr := filepath.EvalSymlinks(filepath.Dir(profile))
		if parentErr != nil {
			return -1, "", parentErr
		}
		resolved = filepath.Join(parent, filepath.Base(profile))
	} else if err != nil {
		return -1, "", err
	}
	path = filepath.Join(resolved, "gh-copilot-attendant", "permission-hook", "events.jsonl")
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/")
	if !filepath.IsAbs(path) || len(parts) < 3 {
		return -1, "", errors.New("invalid log path")
	}
	dir, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, "", err
	}
	success := false
	defer func() {
		if !success {
			_ = unix.Close(dir)
		}
	}()
	for i, part := range parts[:len(parts)-1] {
		next, openErr := unix.Openat(dir, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if errors.Is(openErr, unix.ENOENT) && create && i >= len(parts)-4 {
			if err := unix.Mkdirat(dir, part, 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
				return -1, "", fmt.Errorf("create log directory %q: %w", part, err)
			}
			next, openErr = unix.Openat(dir, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		}
		if openErr != nil {
			return -1, "", fmt.Errorf("open log directory %q: %w", part, openErr)
		}
		if i >= len(parts)-3 {
			var st unix.Stat_t
			if err := unix.Fstat(next, &st); err != nil {
				_ = unix.Close(next)
				return -1, "", err
			}
			if st.Uid != uint32(unix.Geteuid()) || st.Mode&0o077 != 0 {
				_ = unix.Close(next)
				return -1, "", errors.New("unsafe log directory")
			}
			if err := checkPrivateACL(next); err != nil {
				_ = unix.Close(next)
				return -1, "", err
			}
		}
		_ = unix.Close(dir)
		dir = next
	}
	success = true
	return dir, parts[len(parts)-1], nil
}

func validatePrivateLogFile(fd int) error {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0o077 != 0 || st.Uid != uint32(unix.Geteuid()) || st.Nlink != 1 {
		return errors.New("unsafe log file")
	}
	if err := checkPrivateACL(fd); err != nil {
		return err
	}
	return nil
}
