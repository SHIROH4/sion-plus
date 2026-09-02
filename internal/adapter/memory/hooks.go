package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
	"github.com/SHIROH4/sion-plus/internal/port"
)

// LLMHooks wires LLM executors into the MemoryWorker and Compressor.
// Supports both ProviderRegistry (preferred, for task-type routing) and
// direct LLMExecutor (for single-provider setups or testing).
// Call Install() after creating the worker and compressor.
type LLMHooks struct {
	registry port.LLMProviderRegistry // preferred: routes by task type
	executor port.LLMExecutor         // fallback: direct executor

	worker   *MemoryWorker
	compress *Compressor
}

// NewLLMHooks creates a hook installer with a direct executor.
func NewLLMHooks(executor port.LLMExecutor, worker *MemoryWorker, compress *Compressor) *LLMHooks {
	return &LLMHooks{executor: executor, worker: worker, compress: compress}
}

// NewLLMHooksWithRegistry creates a hook installer that routes through ProviderRegistry.
// Each memory task gets the appropriate model: memory tier for extraction/signal/reflection,
// summary tier for compression.
func NewLLMHooksWithRegistry(registry port.LLMProviderRegistry, worker *MemoryWorker, compress *Compressor) *LLMHooks {
	return &LLMHooks{registry: registry, worker: worker, compress: compress}
}

// getExecutor returns the LLM executor for a given task type.
// Registry takes precedence; falls back to direct executor.
func (h *LLMHooks) getExecutor(taskType string) port.LLMExecutor {
	if h.registry != nil {
		exec, _, err := h.registry.GetExecutor(taskType)
		if err == nil {
			return exec
		}
	}
	return h.executor
}

// Install sets all LLM hooks on the worker and compressor.
// Safe to call with nil worker or nil compressor — those hooks are simply skipped.
func (h *LLMHooks) Install() {
	if h.worker != nil {
		h.worker.SetExtractFactsHook(h.extractFacts)
		h.worker.SetDetectSignalsHook(h.detectSignals)
		h.worker.SetReflectAndDiaryHook(h.reflectAndDiary)
	}
	if h.compress != nil {
		h.compress.SetCompressHook(h.compressMessages)
	}
	log.Println("[LLMHooks] installed (extractFacts, detectSignals, reflectAndDiary, compress)")
}

// ── Fact Extraction ────────────────────────────────────────────────

type factExtractionResult struct {
	Facts []struct {
		Entity       string `json:"entity"`
		RelationType string `json:"relation_type"`
		Content      string `json:"content"`
		SourceTier   string `json:"source_tier"`
		Importance   int    `json:"importance"`
	} `json:"facts"`
}

func (h *LLMHooks) extractFacts(ctx context.Context, messages []types.Message) ([]types.FactEntry, error) {
	text := messagesToText(messages)
	prompt := buildFactExtractionPrompt(text)

	exec := h.getExecutor("memory")
	resp, err := exec.Chat(ctx, "", []port.LLMMessage{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("extractFacts LLM: %w", err)
	}

	var result factExtractionResult
	if err := json.Unmarshal([]byte(extractJSON(resp)), &result); err != nil {
		return nil, fmt.Errorf("extractFacts parse: %w (raw: %.200s)", err, resp)
	}

	facts := make([]types.FactEntry, 0, len(result.Facts))
	for _, f := range result.Facts {
		if f.Content == "" || f.Importance < 3 {
			continue
		}
		facts = append(facts, types.FactEntry{
			Entity:       f.Entity,
			RelationType: f.RelationType,
			Content:      f.Content,
			SourceTier:   types.FactSourceTier(f.SourceTier),
			Importance:   f.Importance,
		})
	}
	return facts, nil
}

// ── Signal Detection ───────────────────────────────────────────────

type signalDetectionResult struct {
	Signals []struct {
		EntryID           int64  `json:"entry_id"`
		Type              string `json:"type"`
		SourceFactContent string `json:"source_fact_content"`
	} `json:"signals"`
}

func (h *LLMHooks) detectSignals(ctx context.Context, newFacts, existingFacts []types.FactEntry) ([]SignalResult, error) {
	newText := factsToCompactText(newFacts)
	existingText := factsToCompactText(existingFacts)
	prompt := buildSignalDetectionPrompt(newText, existingText)

	exec := h.getExecutor("signal")
	resp, err := exec.Chat(ctx, "", []port.LLMMessage{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("detectSignals LLM: %w", err)
	}

	var result signalDetectionResult
	if err := json.Unmarshal([]byte(extractJSON(resp)), &result); err != nil {
		return nil, fmt.Errorf("detectSignals parse: %w", err)
	}

	signals := make([]SignalResult, 0, len(result.Signals))
	for _, s := range result.Signals {
		signals = append(signals, SignalResult{
			EntryID: s.EntryID,
			Type:    s.Type,
			Source:  s.SourceFactContent,
		})
	}
	return signals, nil
}

// ── Reflection + Diary ─────────────────────────────────────────────

type reflectAndDiaryResultJSON struct {
	Reflections []struct {
		Text         string `json:"text"`
		Entity       string `json:"entity"`
		RelationType string `json:"relation_type"`
		Importance   int    `json:"importance"`
	} `json:"reflections"`
	Diary *struct {
		Title          string  `json:"title"`
		Summary        string  `json:"summary"`
		EmotionValence float64 `json:"emotion_valence"`
		EmotionArousal float64 `json:"emotion_arousal"`
		EmotionPrimary string  `json:"emotion_primary"`
	} `json:"diary"`
}

func (h *LLMHooks) reflectAndDiary(ctx context.Context, facts []types.FactEntry, msgs []types.Message) (*ReflectAndDiaryResult, error) {
	factText := factsToCompactText(facts)
	msgText := messagesToText(msgs)
	prompt := buildReflectAndDiaryPrompt(factText, msgText)

	exec := h.getExecutor("memory")
	resp, err := exec.Chat(ctx, "", []port.LLMMessage{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("reflectAndDiary LLM: %w", err)
	}

	var result reflectAndDiaryResultJSON
	if err := json.Unmarshal([]byte(extractJSON(resp)), &result); err != nil {
		return nil, fmt.Errorf("reflectAndDiary parse: %w", err)
	}

	out := &ReflectAndDiaryResult{}
	for _, r := range result.Reflections {
		if r.Text == "" {
			continue
		}
		out.Reflections = append(out.Reflections, types.ReflectionEntry{
			Text:         r.Text,
			Entity:       r.Entity,
			RelationType: r.RelationType,
			Status:       "pending",
		})
	}
	if result.Diary != nil && result.Diary.Summary != "" {
		out.Diary = &types.DiaryEntry{
			Title:          result.Diary.Title,
			Summary:        result.Diary.Summary,
			EmotionValence: result.Diary.EmotionValence,
			EmotionArousal: result.Diary.EmotionArousal,
			EmotionPrimary: result.Diary.EmotionPrimary,
		}
	}
	return out, nil
}

// ── Compression ────────────────────────────────────────────────────

type compressionResultJSON struct {
	Summary string `json:"summary"`
}

func (h *LLMHooks) compressMessages(ctx context.Context, messages []types.Message) (string, error) {
	text := messagesToText(messages)
	prompt := buildCompressionPrompt(text)

	exec := h.getExecutor("summary")
	resp, err := exec.Chat(ctx, "", []port.LLMMessage{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return "", fmt.Errorf("compress LLM: %w", err)
	}

	var result compressionResultJSON
	if err := json.Unmarshal([]byte(extractJSON(resp)), &result); err != nil {
		// If JSON parse fails, use raw response as summary (truncated)
		if len(resp) > 300 {
			resp = resp[:300]
		}
		return resp, nil
	}
	return result.Summary, nil
}

// ── Helpers ─────────────────────────────────────────────────────────

func messagesToText(msgs []types.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		role := string(m.Role)
		switch role {
		case "user":
			role = "用户"
		case "assistant":
			role = "AI"
		case "system":
			role = "系统"
		}
		b.WriteString(fmt.Sprintf("[%s] %s\n", role, m.Content))
	}
	return b.String()
}

func factsToCompactText(facts []types.FactEntry) string {
	var b strings.Builder
	for _, f := range facts {
		b.WriteString(fmt.Sprintf("[ID:%d] [%s/%s] [%s] %s\n",
			f.ID, f.Entity, f.RelationType, f.SourceTier, f.Content))
	}
	return b.String()
}

// extractJSON extracts JSON from a response that may be wrapped in markdown fences.
func extractJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	// Strip ```json ... ``` fences
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}
	return raw
}
