package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/SHIROH4/sion-plus/internal/port"
)

// AgentLoopConfig configures the ReAct agent loop.
type AgentLoopConfig struct {
	MaxToolRounds  int           // max tool call iterations (default 10)
	ToolTimeout    time.Duration // per-tool execution timeout (default 30s)
	DangerousApprove func(name string, args map[string]any) bool // nil = auto-approve
}

// DefaultAgentConfig returns sensible defaults.
func DefaultAgentConfig() AgentLoopConfig {
	return AgentLoopConfig{
		MaxToolRounds: 10,
		ToolTimeout:   30 * time.Second,
	}
}

// AgentLoopResult is the output of a complete agent run.
type AgentLoopResult struct {
	Response     string       `json:"response"`
	ToolCalls    []ToolResult `json:"tool_calls,omitempty"`
	Rounds       int          `json:"rounds"`
	Duration     string       `json:"duration"`
	Truncated    bool         `json:"truncated,omitempty"`
}

// AgentLoop executes the ReAct loop:
//
//	while not done:
//	    LLM thinks → may call tools → results injected → repeat
func (r *ToolRegistry) AgentLoop(
	ctx context.Context,
	executor port.LLMExecutor,
	systemPrompt string,
	userMessage string,
	history []port.LLMMessage,
	cfg AgentLoopConfig,
) (*AgentLoopResult, error) {
	if executor == nil {
		return nil, fmt.Errorf("no LLM executor")
	}
	if cfg.MaxToolRounds <= 0 {
		cfg.MaxToolRounds = 10
	}

	start := time.Now()

	// Build initial messages
	messages := make([]port.LLMMessage, 0, len(history)+3)
	if systemPrompt != "" {
		messages = append(messages, port.LLMMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, history...)
	messages = append(messages, port.LLMMessage{Role: "user", Content: userMessage})

	tools := r.Specs()
	var toolResults []ToolResult

	for round := 0; round < cfg.MaxToolRounds; round++ {
		log.Printf("[AgentLoop] round %d/%d", round+1, cfg.MaxToolRounds)

		// Call LLM with tools
		response, err := executor.ChatWithTools(ctx, "", messages, tools,
			func(name, argsJSON string) string {
				// Parse args
				var args map[string]any
				if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
					args = map[string]any{"raw": argsJSON}
				}

				// Approval gate
				tool := r.Get(name)
				if tool != nil && tool.Dangerous && cfg.DangerousApprove != nil {
					if !cfg.DangerousApprove(name, args) {
						return `{"error": "user denied tool execution"}`
					}
				}

				// Execute
				ctx, cancel := context.WithTimeout(ctx, cfg.ToolTimeout)
				defer cancel()

				result := r.Execute(ctx, name, args)
				toolResults = append(toolResults, *result)

				// Format as tool result JSON
				resp, _ := json.Marshal(result)
				return string(resp)
			},
			1,  // maxRounds for ChatWithTools (single round, loop is ours)
			"auto",
		)

		if err != nil {
			return nil, fmt.Errorf("agent loop round %d: %w", round+1, err)
		}

		// If response has no tool calls visible, it's the final answer
		// ChatWithTools returns the final text when no tool calls
		if response != "" {
			return &AgentLoopResult{
				Response:  response,
				ToolCalls: toolResults,
				Rounds:    round + 1,
				Duration:  time.Since(start).String(),
			}, nil
		}

		// Check for max rounds
		if round == cfg.MaxToolRounds-1 {
			return &AgentLoopResult{
				Response:  response,
				ToolCalls: toolResults,
				Rounds:    round + 1,
				Duration:  time.Since(start).String(),
				Truncated: true,
			}, nil
		}
	}

	return &AgentLoopResult{
		Response:  "Max tool rounds reached without resolution.",
		ToolCalls: toolResults,
		Rounds:    cfg.MaxToolRounds,
		Duration:  time.Since(start).String(),
		Truncated: true,
	}, nil
}

// ── Agent System Prompt ───────────────────────────────────────────

const agentSystemPrompt = `你是一个能够使用工具来完成任务的AI助手。

可用工具已通过函数调用(function calling)注册。当需要执行操作时，直接调用对应工具。

工具使用原则：
1. read_file 用于查看文件内容。大文件使用 offset/limit 分页。
2. write_file 用于创建或覆盖文件。会自动创建父目录。
3. edit_file 用于精准修改文件。old_string 必须完全匹配。
4. exec_command 用于执行命令。只能使用白名单内的命令。
5. web_search 用于搜索网络信息。

安全约束：
- 文件操作限制在 ~/.sion/ 和当前项目目录内
- 命令执行限制在白名单内
- 修改文件前请先 read_file 确认内容
- 操作完成后汇报结果

输出要求：
- 使用中文回复
- 简洁明了，不要过度解释
- 操作成功时说明做了什么
- 操作失败时说明原因和建议`

// BuildAgentPrompt returns a system prompt for tool-using sessions.
func BuildAgentPrompt(personality string) string {
	if personality != "" {
		return personality + "\n\n" + agentSystemPrompt
	}
	return agentSystemPrompt
}

// ── Simple tool call (single turn, for ChatOrchestrator) ─────────

// TryToolCall attempts a single-turn tool-assisted response.
// If the LLM doesn't call any tool, it returns the direct response.
// If tools are called, results are injected and the LLM is called once more.
func (r *ToolRegistry) TryToolCall(
	ctx context.Context,
	executor port.LLMExecutor,
	systemPrompt string,
	userMessage string,
	approveFn func(name string, args map[string]any) bool,
) (string, []ToolResult, error) {
	if len(r.List()) == 0 {
		// No tools registered, fallback to normal chat
		resp, err := executor.Chat(ctx, systemPrompt, []port.LLMMessage{
			{Role: "user", Content: userMessage},
		})
		return resp, nil, err
	}

	messages := make([]port.LLMMessage, 0, 3)
	if systemPrompt != "" {
		messages = append(messages, port.LLMMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, port.LLMMessage{Role: "user", Content: userMessage})

	tools := r.Specs()
	var toolResults []ToolResult

	response, err := executor.ChatWithTools(ctx, "", messages, tools,
		func(name, argsJSON string) string {
			var args map[string]any
			json.Unmarshal([]byte(argsJSON), &args)

			tool := r.Get(name)
			if tool != nil && tool.Dangerous && approveFn != nil {
				if !approveFn(name, args) {
					return `{"error": "user denied"}`
				}
			}

			result := r.Execute(ctx, name, args)
			toolResults = append(toolResults, *result)
			resp, _ := json.Marshal(result)
			return string(resp)
		},
		5,
		"auto",
	)

	return strings.TrimSpace(response), toolResults, err
}
