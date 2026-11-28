package permissionhook

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/cli/cli/v2/git"
	"github.com/srz-zumix/go-gh-extension/pkg/gitutil"
)

func currentRepositoryRemote(cwd string) (*url.URL, error) {
	if !filepath.IsAbs(cwd) {
		return nil, errors.New("hook cwd must be absolute")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd, err := gitutil.NewClientWithDir(cwd).Command(ctx, "remote", "get-url", "origin")
	if err != nil {
		return nil, err
	}
	// Inherited Git overrides must not select a repository other than the hook cwd.
	cmd.Env = []string{}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), "GIT_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	remote, err := git.ParseURL(strings.TrimSpace(string(output)))
	if err != nil {
		return nil, err
	}
	if !validRepositoryRemote(remote) {
		return nil, errors.New("origin is not a supported repository URL")
	}
	return remote, nil
}

func validRepositoryRemote(remote *url.URL) bool {
	switch remote.Scheme {
	case "https":
		if remote.Port() != "" && remote.Port() != "443" {
			return false
		}
	case "http":
		if remote.Port() != "" && remote.Port() != "80" {
			return false
		}
	case "ssh", "git":
	default:
		return false
	}
	if remote.Hostname() == "" || remote.Opaque != "" || remote.RawQuery != "" || remote.Fragment != "" ||
		!validRepositoryPath(remote.EscapedPath()) {
		return false
	}
	parts := strings.Split(strings.Trim(remote.Path, "/"), "/")
	if len(parts) != 2 {
		return false
	}
	for i, part := range parts {
		if i == 1 {
			part = strings.TrimSuffix(part, ".git")
		}
		if part == "" || part == "." || part == ".." {
			return false
		}
		for _, c := range part {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._-", c)) {
				return false
			}
		}
	}
	return true
}

func validRepositoryRequestURL(raw string) bool {
	if !validScopedURL(raw) {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && (u.Port() == "" || u.Port() == "443") &&
		validRepositoryPath(u.EscapedPath())
}

// Validate before comparing prefixes; cleaning could hide an escaping path.
func validRepositoryPath(escaped string) bool {
	if !strings.HasPrefix(escaped, "/") {
		return false
	}
	parts := strings.Split(escaped[1:], "/")
	for i, part := range parts {
		if part == "" && i != len(parts)-1 {
			return false
		}
		decoded, err := url.PathUnescape(part)
		if err != nil || decoded == "." || decoded == ".." || strings.ContainsAny(decoded, "/\\%") ||
			strings.ContainsFunc(decoded, unicode.IsControl) {
			return false
		}
	}
	return true
}

func matchesRepositoryURL(remote *url.URL, requested string) bool {
	if remote == nil || !validRepositoryRemote(remote) || !validRepositoryRequestURL(requested) {
		return false
	}
	u, _ := url.Parse(requested)
	host := strings.ToLower(remote.Hostname())
	repoPath := "/" + strings.TrimSuffix(strings.Trim(remote.Path, "/"), ".git")
	requestHost := strings.ToLower(u.Hostname())
	if requestHost == host && repositoryPathMatches(u.Path, repoPath) {
		return true
	}
	if host == "github.com" {
		return requestHost == "api.github.com" && repositoryPathMatches(u.Path, "/repos"+repoPath) ||
			requestHost == "raw.githubusercontent.com" && repositoryPathMatches(u.Path, repoPath)
	}
	return requestHost == host && repositoryPathMatches(u.Path, "/api/v3/repos"+repoPath) ||
		requestHost == "raw."+host && repositoryPathMatches(u.Path, repoPath)
}

func repositoryPathMatches(requested, root string) bool {
	if len(requested) < len(root) || !strings.EqualFold(requested[:len(root)], root) {
		return false
	}
	return len(requested) == len(root) || requested[len(root)] == '/'
}
