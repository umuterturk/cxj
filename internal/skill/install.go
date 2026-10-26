// Package skill embeds and installs the agent SKILL.md for supported harnesses.
package skill

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// Name is the skill directory name on every harness.
const Name = "cxj"

//go:embed SKILL.md
var markdown []byte

// Markdown is the installable skill document.
func Markdown() []byte { return append([]byte(nil), markdown...) }

// HomeDir prefers HOME / USERPROFILE so tests can redirect installs.
func HomeDir() (string, error) {
	if h := os.Getenv("HOME"); h != "" {
		return h, nil
	}
	if h := os.Getenv("USERPROFILE"); h != "" {
		return h, nil
	}
	return os.UserHomeDir()
}

// AgentsSkillDir is the canonical skill location (.agents/skills/<name>).
func AgentsSkillDir(root string) string {
	return filepath.Join(root, ".agents", "skills", Name)
}

// ClaudeSkillDir is the Claude-compatible skill path (.claude/skills/<name>).
func ClaudeSkillDir(root string) string {
	return filepath.Join(root, ".claude", "skills", Name)
}

// InstallLocal writes SKILL.md under root/.agents/skills/cxj and symlinks
// root/.claude/skills/cxj to it (Cursor/OpenCode/Codex read .agents; Claude needs .claude).
func InstallLocal(root string) ([]string, error) {
	if root == "" {
		return nil, fmt.Errorf("project root is empty")
	}
	return installAt(root, filepath.Join("..", "..", ".agents", "skills", Name))
}

// InstallGlobal writes SKILL.md under $HOME/.agents/skills/cxj and symlinks
// $HOME/.claude/skills/cxj to it.
func InstallGlobal(home string) ([]string, error) {
	if home == "" {
		return nil, fmt.Errorf("home directory is empty")
	}
	return installAt(home, filepath.Join("..", "..", ".agents", "skills", Name))
}

func installAt(root, claudeRelTarget string) ([]string, error) {
	agentsDir := AgentsSkillDir(root)
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", agentsDir, err)
	}
	skillPath := filepath.Join(agentsDir, "SKILL.md")
	if err := os.WriteFile(skillPath, markdown, 0644); err != nil {
		return nil, fmt.Errorf("write %s: %w", skillPath, err)
	}

	claudeLink := ClaudeSkillDir(root)
	if err := os.MkdirAll(filepath.Dir(claudeLink), 0755); err != nil {
		return []string{skillPath}, fmt.Errorf("mkdir %s: %w", filepath.Dir(claudeLink), err)
	}
	if err := ensureSymlink(claudeLink, claudeRelTarget); err != nil {
		return []string{skillPath}, err
	}
	return []string{skillPath, claudeLink}, nil
}

// ensureSymlink makes link a symlink to target (relative path stored in the link).
// Replaces an existing symlink or directory at link.
func ensureSymlink(link, target string) error {
	if fi, err := os.Lstat(link); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			cur, err := os.Readlink(link)
			if err == nil && cur == target {
				return nil
			}
			if err := os.Remove(link); err != nil {
				return fmt.Errorf("remove symlink %s: %w", link, err)
			}
		} else {
			if err := os.RemoveAll(link); err != nil {
				return fmt.Errorf("replace %s: %w", link, err)
			}
		}
	}
	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", link, target, err)
	}
	return nil
}
