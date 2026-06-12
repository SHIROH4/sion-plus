package tool

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// RegisterFileTools adds read_file, write_file, and edit_file to the registry.
func (r *ToolRegistry) RegisterFileTools() {
	r.Register(&ToolDef{
		Name:        "read_file",
		Description: "Read the contents of a file. Returns the file content with line numbers. Use offset/limit for large files.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "description": "Absolute path to the file"},
				"offset": map[string]any{"type": "integer", "description": "Line number to start reading from (1-based, optional)"},
				"limit":  map[string]any{"type": "integer", "description": "Maximum number of lines to read (optional, default 200)"},
			},
			"required": []string{"path"},
		},
		Handler: handleReadFile,
	})

	r.Register(&ToolDef{
		Name:        "write_file",
		Description: "Write content to a file. Creates parent directories if needed. Overwrites existing files.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "Absolute path to write to"},
				"content": map[string]any{"type": "string", "description": "Content to write"},
			},
			"required": []string{"path", "content"},
		},
		Handler:   handleWriteFile,
		Dangerous: true,
	})

	r.Register(&ToolDef{
		Name:        "edit_file",
		Description: "Perform exact string replacement in an existing file. old_string must match exactly (including whitespace).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":       map[string]any{"type": "string", "description": "Absolute path to the file"},
				"old_string": map[string]any{"type": "string", "description": "Text to replace (must match exactly)"},
				"new_string": map[string]any{"type": "string", "description": "Replacement text"},
			},
			"required": []string{"path", "old_string", "new_string"},
		},
		Handler:   handleEditFile,
		Dangerous: true,
	})
}

func handleReadFile(ctx context.Context, args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	safePath, err := validatePath(path)
	if err != nil {
		return "", err
	}
	if !pathExists(safePath) {
		return "", fmt.Errorf("file not found: %s", safePath)
	}

	data, err := os.ReadFile(safePath)
	if err != nil {
		return "", fmt.Errorf("read error: %w", err)
	}

	offset := intArg(args, "offset", 1)
	limit := intArg(args, "limit", 200)

	lines := strings.Split(string(data), "\n")
	if offset > len(lines) {
		offset = len(lines)
	}
	end := offset + limit - 1
	if end > len(lines) {
		end = len(lines)
	}

	var b strings.Builder
	for i := offset - 1; i < end; i++ {
		fmt.Fprintf(&b, "%6d\t%s\n", i+1, lines[i])
	}
	return b.String(), nil
}

func handleWriteFile(ctx context.Context, args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)

	safePath, err := validatePath(path)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(strings.TrimSuffix(safePath, "/"+strings.TrimLeft(safePath, "/")), 0755); err != nil {
		// Try simpler approach
	}
	dir := safePath[:strings.LastIndex(safePath, "/")]
	if dir != "" {
		os.MkdirAll(dir, 0755)
	}

	if err := os.WriteFile(safePath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write error: %w", err)
	}
	return fmt.Sprintf("Wrote %d bytes to %s", len(content), safePath), nil
}

func handleEditFile(ctx context.Context, args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	oldStr, _ := args["old_string"].(string)
	newStr, _ := args["new_string"].(string)

	safePath, err := validatePath(path)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(safePath)
	if err != nil {
		return "", fmt.Errorf("read error: %w", err)
	}

	content := string(data)
	count := strings.Count(content, oldStr)
	if count == 0 {
		return "", fmt.Errorf("old_string not found in file")
	}
	if count > 1 {
		return "", fmt.Errorf("old_string matches %d times (must be unique for safe edit)", count)
	}

	newContent := strings.Replace(content, oldStr, newStr, 1)
	if err := os.WriteFile(safePath, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("write error: %w", err)
	}
	return fmt.Sprintf("Replaced 1 occurrence in %s", safePath), nil
}

func intArg(args map[string]any, key string, def int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return def
}
