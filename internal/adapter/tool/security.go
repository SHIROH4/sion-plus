package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ── Path Sandbox ──────────────────────────────────────────────────

// allowedPaths lists directories where file operations are permitted.
var allowedPaths = []string{}

// InitAllowedPaths sets the allowed directories for file operations.
func InitAllowedPaths(homeDir, projectDir string) {
	allowedPaths = []string{
		homeDir,
		filepath.Join(homeDir, ".sion"),
		filepath.Join(homeDir, "Desktop"),
		filepath.Join(homeDir, "Documents"),
		filepath.Join(homeDir, "Downloads"),
		projectDir,
	}
}

// validatePath checks if a path is within allowed directories.
func validatePath(target string) (string, error) {
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	clean := filepath.Clean(abs)

	for _, allowed := range allowedPaths {
		if strings.HasPrefix(clean, allowed+string(os.PathSeparator)) || clean == allowed {
			return clean, nil
		}
	}
	return "", fmt.Errorf("path %q is outside allowed directories", clean)
}

// pathExists checks if a file or directory exists.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ── Command Whitelist ────────────────────────────────────────────

// allowedCommands is the whitelist of executable commands.
var allowedCommands = map[string]bool{
	"ls": true, "cat": true, "head": true, "tail": true, "wc": true,
	"grep": true, "find": true, "pwd": true, "which": true, "echo": true,
	"date": true, "whoami": true, "uname": true, "ps": true,
	"go": true, "git": true, "make": true, "cargo": true, "rustc": true,
	"python3": true, "python": true, "node": true, "npm": true,
	"curl": true, "wget": true, "brew": true,
	"df": true, "du": true, "top": true, "htop": true,
	"docker": true, "kubectl": true,
	"open": true, "code": true, "vim": true, "nano": true,
}

// validateCommand checks if a command is in the whitelist.
// Returns the command name and any error.
func validateCommand(cmd string) (string, error) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}
	base := filepath.Base(parts[0])
	if !allowedCommands[base] {
		return "", fmt.Errorf("command %q is not in the allowed list", base)
	}
	return base, nil
}

// dangerousCommands lists commands that ALWAYS require user approval.
var dangerousCommands = map[string]bool{
	"rm": true, "mv": true, "cp": true, "chmod": true, "chown": true,
	"sudo": true, "kill": true, "shutdown": true, "reboot": true,
	"dd": true, "mkfs": true, "mount": true, "umount": true,
}

// IsDangerous checks if a command needs user confirmation.
func IsDangerous(cmd string) bool {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		// Check for pipe to dangerous commands
		if dangerousCommands[part] {
			return true
		}
	}
	return false
}

// ── Network Safety ──────────────────────────────────────────────

// isLocalhost checks if a URL points to localhost/loopback.
func isLocalhost(url string) bool {
	return strings.Contains(url, "127.0.0.1") ||
		strings.Contains(url, "localhost") ||
		strings.Contains(url, "[::1]")
}
