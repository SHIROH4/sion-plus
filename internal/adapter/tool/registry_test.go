package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestToolRegistryRegister(t *testing.T) {
	r := NewToolRegistry()
	r.Register(&ToolDef{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			return "ok", nil
		},
	})

	if r.Get("test_tool") == nil {
		t.Fatal("tool not found after register")
	}
	if len(r.List()) != 1 {
		t.Errorf("expected 1 tool, got %d", len(r.List()))
	}
}

func TestToolRegistryExecute(t *testing.T) {
	r := NewToolRegistry()
	r.Register(&ToolDef{
		Name:        "echo",
		Description: "Echoes input",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string"},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			text, _ := args["text"].(string)
			return "echo: " + text, nil
		},
	})

	result := r.Execute(context.Background(), "echo", map[string]any{"text": "hello"})
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if result.Output != "echo: hello" {
		t.Errorf("expected 'echo: hello', got %q", result.Output)
	}
}

func TestToolRegistryNotFound(t *testing.T) {
	r := NewToolRegistry()
	result := r.Execute(context.Background(), "no_such_tool", nil)
	if result.Success {
		t.Error("expected failure for unknown tool")
	}
}

func TestToolRegistrySpecs(t *testing.T) {
	r := NewToolRegistry()
	r.Register(&ToolDef{
		Name:        "tool_a",
		Description: "First tool",
		Parameters:  map[string]any{},
		Handler:     func(ctx context.Context, args map[string]any) (string, error) { return "", nil },
	})
	r.Register(&ToolDef{
		Name:        "tool_b",
		Description: "Second tool",
		Parameters:  map[string]any{},
		Handler:     func(ctx context.Context, args map[string]any) (string, error) { return "", nil },
	})

	specs := r.Specs()
	if len(specs) != 2 {
		t.Errorf("expected 2 specs, got %d", len(specs))
	}
}

func TestFileTools(t *testing.T) {
	r := NewToolRegistry()
	r.RegisterFileTools()

	dir := t.TempDir()
	InitAllowedPaths(dir, dir)

	// Test write_file
	result := r.Execute(context.Background(), "write_file", map[string]any{
		"path":    filepath.Join(dir, "test.txt"),
		"content": "hello world",
	})
	if !result.Success {
		t.Fatalf("write_file: %s", result.Error)
	}

	// Test read_file
	result2 := r.Execute(context.Background(), "read_file", map[string]any{
		"path": filepath.Join(dir, "test.txt"),
	})
	if !result2.Success {
		t.Fatalf("read_file: %s", result2.Error)
	}
	if !contains(result2.Output, "hello world") {
		t.Errorf("read output: %s", result2.Output)
	}

	// Test edit_file
	result3 := r.Execute(context.Background(), "edit_file", map[string]any{
		"path":       filepath.Join(dir, "test.txt"),
		"old_string": "hello world",
		"new_string": "hello sion",
	})
	if !result3.Success {
		t.Fatalf("edit_file: %s", result3.Error)
	}

	// Verify edit
	result4 := r.Execute(context.Background(), "read_file", map[string]any{
		"path": filepath.Join(dir, "test.txt"),
	})
	if !contains(result4.Output, "hello sion") {
		t.Errorf("edit verify: %s", result4.Output)
	}
}

func TestBashTool(t *testing.T) {
	r := NewToolRegistry()
	r.RegisterBashTool()

	result := r.Execute(context.Background(), "exec_command", map[string]any{
		"command": "echo hello from bash",
	})
	if !result.Success {
		t.Fatalf("bash tool: %s", result.Error)
	}
	if !contains(result.Output, "hello from bash") {
		t.Errorf("bash output: %s", result.Output)
	}
}

func TestBashDangerousBlocked(t *testing.T) {
	r := NewToolRegistry()
	r.RegisterBashTool()

	result := r.Execute(context.Background(), "exec_command", map[string]any{
		"command": "rm -rf /tmp/test",
	})
	if result.Success {
		t.Error("dangerous command should be blocked")
	}
}

func TestBashNotWhitelisted(t *testing.T) {
	r := NewToolRegistry()
	r.RegisterBashTool()

	result := r.Execute(context.Background(), "exec_command", map[string]any{
		"command": "nc -l 8080",
	})
	if result.Success {
		t.Error("non-whitelisted command should be blocked")
	}
}

func TestPathSandbox(t *testing.T) {
	dir := t.TempDir()
	InitAllowedPaths(dir, dir)

	// Allowed path
	_, err := validatePath(filepath.Join(dir, "test.txt"))
	if err != nil {
		t.Errorf("allowed path should pass: %v", err)
	}

	// Blocked path
	_, err = validatePath("/etc/passwd")
	if err == nil {
		t.Error("blocked path should fail")
	}
}

func TestSearchTool(t *testing.T) {
	r := NewToolRegistry()
	r.RegisterSearchTool()

	result := r.Execute(context.Background(), "web_search", map[string]any{
		"query":       "golang concurrency patterns",
		"max_results": float64(3),
	})
	if !result.Success {
		t.Logf("search failed (may be offline): %s", result.Error)
	}
	t.Logf("search result: %.100s...", result.Output)
}

func TestToolRegistryStats(t *testing.T) {
	r := NewToolRegistry()
	r.Register(&ToolDef{
		Name:    "s",
		Handler: func(ctx context.Context, args map[string]any) (string, error) { return "ok", nil },
	})

	r.Execute(context.Background(), "s", nil)
	r.Execute(context.Background(), "s", nil)
	r.Execute(context.Background(), "nonexistent", nil)

	calls, errors := r.Stats()
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
	if errors != 1 {
		t.Errorf("expected 1 error, got %d", errors)
	}
}

func TestAgentLoopPrompt(t *testing.T) {
	prompt := BuildAgentPrompt("你是一只猫娘。")
	if !contains(prompt, "猫娘") {
		t.Error("prompt should contain personality")
	}
	if !contains(prompt, "read_file") {
		t.Error("prompt should mention tools")
	}
}

func TestToolRegistryUnregister(t *testing.T) {
	r := NewToolRegistry()
	r.Register(&ToolDef{Name: "t", Handler: func(ctx context.Context, args map[string]any) (string, error) { return "", nil }})
	r.Unregister("t")
	if r.Get("t") != nil {
		t.Error("tool should be gone")
	}
}

func TestInitAllowedPaths(t *testing.T) {
	InitAllowedPaths("/home/user", "/home/user/project")
	if len(allowedPaths) < 2 {
		t.Error("allowed paths not initialized")
	}
}

func TestDangerousCmdDetection(t *testing.T) {
	if !IsDangerous("rm -rf /tmp/test") {
		t.Error("rm should be dangerous")
	}
	if !IsDangerous("sudo ls") {
		t.Error("sudo should be dangerous")
	}
	if IsDangerous("ls -la") {
		t.Error("ls should not be dangerous")
	}
}

// cleanup test files after tests
func init() {
	// Ensure allowedPaths is clean for tests
	allowedPaths = nil
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Prevent init() from interfering
var _ = os.DevNull
