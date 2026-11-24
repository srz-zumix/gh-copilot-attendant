package permissionhook

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// EnvironmentAssignmentConfig controls leading Bash assignments on allowed commands.
type EnvironmentAssignmentConfig struct {
	Mode         string   `yaml:"mode"`
	Allow        []string `yaml:"allow"`
	Deny         []string `yaml:"deny"`
	TrustedHosts []string `yaml:"trustedHosts"`
}

var defaultAllowedEnvironment = map[string]bool{
	"GH_TOKEN":                true,
	"GITHUB_TOKEN":            true,
	"GH_ENTERPRISE_TOKEN":     true,
	"GITHUB_ENTERPRISE_TOKEN": true,
	"NO_COLOR":                true,
	"CLICOLOR":                true,
	"CLICOLOR_FORCE":          true,
	"GH_COLOR_LABELS":         true,
	"GH_ACCESSIBLE_COLORS":    true,
	"GH_MDWIDTH":              true,
	"GH_SPINNER_DISABLED":     true,
}

var defaultDeniedEnvironment = []string{
	"BASH_ENV", "ENV", "SHELLOPTS", "BASHOPTS", "IFS", "CDPATH",
	"LD_*", "DYLD_*", "GIT_*", "GCONV_PATH", "GIO_EXTRA_MODULES",
	"GLIBC_TUNABLES", "MALLOC_*",
	"GH_HOST", "GH_REPO", "GH_CONFIG_DIR", "GH_PATH", "GH_EDITOR",
	"GH_BROWSER", "GH_PAGER", "GH_DEBUG", "GH_FORCE_TTY",
	"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
	"EDITOR", "VISUAL", "PAGER", "BROWSER", "LESSOPEN", "LESSCLOSE",
	"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY",
	"http_proxy", "https_proxy", "all_proxy", "no_proxy",
	"SSL_CERT_FILE", "SSL_CERT_DIR", "CURL_CA_BUNDLE", "REQUESTS_CA_BUNDLE",
	"NODE_OPTIONS", "NODE_PATH", "PYTHONPATH", "PYTHONHOME",
	"PERL5OPT", "PERL5LIB", "RUBYOPT", "RUBYLIB",
	"JAVA_TOOL_OPTIONS", "JDK_JAVA_OPTIONS", "_JAVA_OPTIONS",
	"GOFLAGS", "GOCACHEPROG", "GOAUTH", "GOENV", "GOROOT", "GOTOOLCHAIN",
	"CC", "CXX", "FC", "AR", "PKG_CONFIG", "GCCGO", "GCCGOTOOLDIR",
	"CGO_CFLAGS*", "CGO_CPPFLAGS*", "CGO_CXXFLAGS*", "CGO_FFLAGS*", "CGO_LDFLAGS*",
}

func (c EnvironmentAssignmentConfig) validate() error {
	switch c.Mode {
	case "", "allowlist", "denylist":
	default:
		return fmt.Errorf("invalid mode %q: expected allowlist or denylist", c.Mode)
	}
	for _, list := range []struct {
		name     string
		patterns []string
	}{{"allow", c.Allow}, {"deny", c.Deny}} {
		for _, pattern := range list.patterns {
			if pattern == "" || strings.Contains(pattern, "/") {
				return fmt.Errorf("%s: invalid environment pattern %q", list.name, pattern)
			}
			if _, err := path.Match(pattern, ""); err != nil {
				return fmt.Errorf("%s: invalid environment pattern %q: %w", list.name, pattern, err)
			}
			if list.name == "allow" && pattern == "PATH" {
				return fmt.Errorf("allow: PATH cannot be allowed")
			}
		}
	}
	for _, host := range c.TrustedHosts {
		if _, ok := normalizeTrustedHost(host); !ok {
			return fmt.Errorf("trustedHosts: invalid host %q", host)
		}
	}
	return nil
}

func (c EnvironmentAssignmentConfig) allows(name string) bool {
	if name == "PATH" {
		return false
	}
	if denied, valid := matchesEnvironmentPatterns(c.Deny, name); !valid || denied {
		return false
	}
	if allowed, valid := matchesEnvironmentPatterns(c.Allow, name); !valid {
		return false
	} else if allowed {
		return true
	}
	switch c.Mode {
	case "", "allowlist":
		return defaultAllowedEnvironment[name]
	case "denylist":
		denied, valid := matchesEnvironmentPatterns(defaultDeniedEnvironment, name)
		return valid && !denied
	default:
		return false
	}
}

func matchesEnvironmentPatterns(patterns []string, name string) (bool, bool) {
	matched := false
	for _, pattern := range patterns {
		ok, err := path.Match(pattern, name)
		if err != nil {
			return false, false
		}
		matched = matched || ok
	}
	return matched, true
}

func (c EnvironmentAssignmentConfig) trustsHost(raw string) bool {
	host, ok := normalizeTrustedHost(raw)
	if !ok {
		return false
	}
	if host == "github.com" || host == "api.github.com" {
		return true
	}
	for _, configured := range c.TrustedHosts {
		trusted, ok := normalizeTrustedHost(configured)
		if !ok {
			return false
		}
		if host == trusted {
			return true
		}
	}
	return false
}

func normalizeTrustedHost(raw string) (string, bool) {
	if raw == "" || strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, `/\?#@%`) {
		return "", false
	}
	u, err := url.Parse("https://" + raw)
	if err != nil || u.Host != raw || u.Hostname() == "" || u.User != nil {
		return "", false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	} else {
		if len(host) > 253 {
			return "", false
		}
		for _, label := range strings.Split(host, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return "", false
			}
			for _, c := range label {
				validChar := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-'
				if !validChar {
					return "", false
				}
			}
		}
	}
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", false
		}
		if n != 443 {
			return net.JoinHostPort(host, strconv.Itoa(n)), true
		}
	}
	return host, true
}
