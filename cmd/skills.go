package cmd

import (
	"io/fs"

	"github.com/srz-zumix/gh-copilot-attendant/version"
	"github.com/srz-zumix/go-gh-extension/pkg/skillsmith"
)

func RegisterSkillsCmd(skillsFS fs.FS) {
	rootCmd.AddCommand(skillsmith.NewSkillsCmd("gh-copilot-attendant", version.Version, skillsFS))
}
