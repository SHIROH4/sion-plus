package tool

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// RegisterBashTool adds exec_command to the registry.
func (r *ToolRegistry) RegisterBashTool() {
	r.Register(&ToolDef{
		Name:        "exec_command",
		Description: "Execute a shell command. Only whitelisted commands are allowed. Use for: running code, git operations, file searches, system info. Do NOT use for destructive operations (rm, sudo, etc) — those are blocked.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The shell command to execute. Must start with a whitelisted command.",
				},
				"workdir": map[string]any{
					"type":        "string",
					"description": "Working directory for the command (optional)",
				},
				"timeout_sec": map[string]any{
					"type":        "integer",
					"description": "Timeout in seconds (optional, default 30, max 120)",
				},
			},
			"required": []string{"command"},
		},
		Handler:   handleBash,
		Dangerous: true,
		Sandboxed: true,
	})
}

func handleBash(ctx context.Context, args map[string]any) (string, error) {
	cmdStr, _ := args["command"].(string)
	if cmdStr == "" {
		return "", fmt.Errorf("command is required")
	}

	// Security check
	if IsDangerous(cmdStr) {
		return "", fmt.Errorf("command contains dangerous operations (rm, sudo, etc). Operation blocked.")
	}

	baseCmd, err := validateCommand(cmdStr)
	if err != nil {
		return "", err
	}

	timeoutSec := intArg(args, "timeout_sec", 30)
	if timeoutSec > 120 {
		timeoutSec = 120
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)

	// Set working directory
	if workdir, ok := args["workdir"].(string); ok && workdir != "" {
		if safeDir, err := validatePath(workdir); err == nil {
			cmd.Dir = safeDir
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err = cmd.Run()
	elapsed := time.Since(start)

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += "[stderr]\n" + stderr.String()
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return output, fmt.Errorf("command timed out after %v", elapsed)
		}
		return output, fmt.Errorf("%s: %w (took %v)", baseCmd, err, elapsed)
	}

	if output == "" {
		output = fmt.Sprintf("(command completed successfully in %v, no output)", elapsed)
	}

	if len(output) > maxOutputLen {
		output = truncateOutput(output)
		output += "\n(command output truncated)"
	}

	return strings.TrimSpace(output), nil
}
