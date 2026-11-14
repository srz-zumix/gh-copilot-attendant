package permissionhook

import (
	"io"
)

// Run handles one permissionRequest hook invocation: it loads the config at configPath
// (resolved via ResolveConfigPath), reads the hook payload from r, and writes the
// decision to w. A missing config file produces no decision.
func Run(r io.Reader, w io.Writer, configPath string) error {
	p, err := ResolveConfigPath(configPath)
	if err != nil {
		return err
	}
	cfg, found, err := LoadConfig(p)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	req, err := ParseRequest(r)
	if err != nil {
		return err
	}
	return WriteDecision(w, Decide(cfg, req))
}
