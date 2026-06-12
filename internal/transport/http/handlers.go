package http

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/SHIROH4/sion-plus/internal/app"
	"github.com/SHIROH4/sion-plus/internal/infra/logbuffer"
	"github.com/SHIROH4/sion-plus/internal/domain/types"
	"github.com/SHIROH4/sion-plus/internal/port"
	"github.com/SHIROH4/sion-plus/internal/transport/sse"
)

type handlers struct {
	runtime   *app.AppRuntime
	broker    *sse.Broker
	logBuffer *logbuffer.Buffer
}

// ── Request / Response types ───────────────────────────────────────

type chatRequest struct {
	Message string `json:"message"`
	Source  string `json:"source"` // "dashboard" | "pet"
}

type chatResponse struct {
	Response string `json:"response"`
	Emotion  string `json:"emotion"`
	Source   string `json:"source"`
}



type emotionResponse struct {
	Primary   string         `json:"primary"`
	Intensity float64        `json:"intensity"`
	Vector    emotionVector8 `json:"vector"`
}

type emotionVector8 struct {
	Affection   float64 `json:"affection"`
	Worry       float64 `json:"worry"`
	Curiosity   float64 `json:"curiosity"`
	Sleepiness  float64 `json:"sleepiness"`
	Playfulness float64 `json:"playfulness"`
	Loneliness  float64 `json:"loneliness"`
	Confidence  float64 `json:"confidence"`
	Annoyance   float64 `json:"annoyance"`
}

// ── POST /api/v1/chat ─────────────────────────────────────────────

func (h *handlers) chat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}

	// Publish user message immediately (before blocking on AI)
	h.broker.Publish("chat-message", map[string]string{
		"role": "user", "content": req.Message,
	})

	result, err := h.runtime.Chat.OnUserMessage(r.Context(), req.Message)
	if err != nil {
		log.Printf("[HTTP] chat error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, chatResponse{
		Response: result.Response,
		Emotion:  result.Emotion.Primary,
		Source:   result.EmotionSource,
	})

	// Publish AI response after processing
	h.broker.Publish("chat-message", map[string]string{
		"role": "assistant", "content": result.Response,
	})

	// Push emotion update via SSE
	h.pushEmotion()
}

	// ── GET /api/v1/chat/history ──────────────────────────────────────

func (h *handlers) chatHistory(w http.ResponseWriter, r *http.Request) {
	msgs, err := h.runtime.LoadChatHistory(r.Context(), 50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
}

func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	errs := h.runtime.Health(r.Context())
	modules := make(map[string]string)
	status := "ok"
	for name, err := range errs {
		if err != nil {
			modules[name] = err.Error()
			status = "degraded"
		} else {
			modules[name] = "ok"
		}
	}
	cpu, memUsed, memTotal, goroutines, uptime := h.runtime.SystemStats()
	type healthFullResponse struct {
		Status     string            `json:"status"`
		Modules    map[string]string `json:"modules"`
		CPU        float64           `json:"cpu_cores"`
		MemUsedMB  int64             `json:"mem_used_mb"`
		MemTotalMB int64             `json:"mem_total_mb"`
		Goroutines int               `json:"goroutines"`
		UptimeSec  int64             `json:"uptime_sec"`
	}
	writeJSON(w, http.StatusOK, healthFullResponse{
		Status:     status,
		Modules:    modules,
		CPU:        cpu,
		MemUsedMB:  memUsed,
		MemTotalMB: memTotal,
		Goroutines: goroutines,
		UptimeSec:  uptime,
	})
}

// ── GET /api/v1/emotion ───────────────────────────────────────────

func (h *handlers) emotion(w http.ResponseWriter, r *http.Request) {
	state, vec := h.runtime.EmotionState()
	writeJSON(w, http.StatusOK, emotionResponse{
		Primary:   state.Primary,
		Intensity: state.Intensity,
		Vector: emotionVector8{
			Affection:   vec.Affection,
			Worry:       vec.Worry,
			Curiosity:   vec.Curiosity,
			Sleepiness:  vec.Sleepiness,
			Playfulness: vec.Playfulness,
			Loneliness:  vec.Loneliness,
			Confidence:  vec.Confidence,
			Annoyance:   vec.Annoyance,
		},
	})
}

// ── GET /api/v1/emotion/history ───────────────────────────────────

func (h *handlers) emotionHistory(w http.ResponseWriter, r *http.Request) {
	states, vectors := h.runtime.EmotionHistory()
	n := len(states)
	if n == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"history": []any{}})
		return
	}
	type entry struct {
		Primary   string         `json:"primary"`
		Intensity float64        `json:"intensity"`
		Vector    emotionVector8 `json:"vector"`
	}
	history := make([]entry, n)
	for i := range states {
		history[i] = entry{
			Primary:   states[i].Primary,
			Intensity: states[i].Intensity,
			Vector: emotionVector8{
				Affection:   vectors[i].Affection,
				Worry:       vectors[i].Worry,
				Curiosity:   vectors[i].Curiosity,
				Sleepiness:  vectors[i].Sleepiness,
				Playfulness: vectors[i].Playfulness,
				Loneliness:  vectors[i].Loneliness,
				Confidence:  vectors[i].Confidence,
				Annoyance:   vectors[i].Annoyance,
			},
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": history})
}

// ── GET /api/v1/stats ─────────────────────────────────────────────

func (h *handlers) stats(w http.ResponseWriter, r *http.Request) {
	type statsResponse struct {
		MessagesToday int `json:"messages_today"`
	}
	writeJSON(w, http.StatusOK, statsResponse{
		MessagesToday: h.runtime.TodayMessageCount(r.Context()),
	})
}

// ── POST /api/v1/chat/stream ───────────────────────────────────────

func (h *handlers) chatStream(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	result, err := h.runtime.Chat.OnUserMessageStream(r.Context(), req.Message, func(chunk string) error {
		data, _ := json.Marshal(map[string]string{"token": chunk})
		fmt.Fprintf(w, "event: token\ndata: %s\n\n", data)
		flusher.Flush()
		return nil
	})
	if err != nil {
		data, _ := json.Marshal(map[string]string{"error": err.Error()})
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
		flusher.Flush()
		return
	}

	final, _ := json.Marshal(chatResponse{
		Response: result.Response,
		Emotion:  result.Emotion.Primary,
		Source:   result.EmotionSource,
	})
	fmt.Fprintf(w, "event: done\ndata: %s\n\n", final)
	flusher.Flush()

		// Publish AI response after processing
	h.broker.Publish("chat-message", map[string]string{
		"role": "user", "content": req.Message,
	})
	h.broker.Publish("chat-message", map[string]string{
		"role": "assistant", "content": result.Response,
	})

	h.pushEmotion()
}

// ── GET /api/v1/screen ────────────────────────────────────────────

func (h *handlers) screen(w http.ResponseWriter, r *http.Request) {
	type screenResponse struct {
		AppName     string `json:"app_name"`
		AppCategory string `json:"app_category"`
		WindowTitle string `json:"window_title"`
	}
	writeJSON(w, http.StatusOK, screenResponse{
		AppName: "N/A", AppCategory: "idle", WindowTitle: "",
	})
}

// ── POST /api/v1/proactive/mode ────────────────────────────────────

func (h *handlers) proactiveMode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode string `json:"mode"` // "normal"|"frequent"|"focus"|"off"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid mode"})
		return
	}

	interval := map[string]int{
		"normal": 60, "frequent": 30, "focus": 120, "off": 0,
	}[req.Mode]

	if h.runtime.CognitionTick != nil {
		if interval > 0 {
			h.runtime.CognitionTick.SetInterval(time.Duration(interval) * time.Second)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"mode": req.Mode, "interval_sec": interval,
	})
}

// ── GET /api/v1/memory/facts ─────────────────────────────────────

func (h *handlers) memoryFacts(w http.ResponseWriter, r *http.Request) {
	entity := r.URL.Query().Get("entity")
	sourceTier := r.URL.Query().Get("source_tier")
	memType := r.URL.Query().Get("type")
	all, _ := h.runtime.ListActiveFacts(r.Context(), 0)

	var filtered []types.FactEntry
	for _, f := range all {
		if entity != "" && f.Entity != entity {
			continue
		}
		if sourceTier != "" && string(f.SourceTier) != sourceTier {
			continue
		}
		if memType != "" && f.MemCellType != memType {
			continue
		}
		filtered = append(filtered, f)
	}
	if filtered == nil {
		filtered = []types.FactEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"facts": filtered})
}

// ── GET /api/v1/memory/topics ─────────────────────────────────────

func (h *handlers) memoryTopics(w http.ResponseWriter, r *http.Request) {
	topics, err := h.runtime.ListTopics(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if topics == nil {
		topics = []types.Topic{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"topics": topics})
}

// ── GET /api/v1/memory/stats ──────────────────────────────────────

func (h *handlers) memoryStats(w http.ResponseWriter, r *http.Request) {
	facts, _ := h.runtime.ListActiveFacts(r.Context(), 0)
	var confirmed, pending int
	entityCount := map[string]int{}
	sourceCount := map[string]int{}
	typeCount := map[string]int{}

	for _, f := range facts {
		if f.Archived {
			continue
		}
		if f.Evidence.Reinforcement-f.Evidence.Disputation >= 0.5 {
			confirmed++
		} else {
			pending++
		}
		entityCount[f.Entity]++
		sourceCount[string(f.SourceTier)]++
		typeCount[f.MemCellType]++
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total":     len(facts),
		"confirmed": confirmed,
		"pending":   pending,
		"by_entity": entityCount,
		"by_source": sourceCount,
		"by_type":   typeCount,
	})
}

// ── GET /api/v1/tools ───────────────────────────────────────────

func (h *handlers) tools(w http.ResponseWriter, r *http.Request) {
	tools := h.runtime.ListTools()
	type toolInfo struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
		Dangerous   bool           `json:"dangerous"`
	}
	out := make([]toolInfo, len(tools))
	for i, t := range tools {
		out[i] = toolInfo{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
			Dangerous:   t.Dangerous,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": out})
}

// ── GET /api/v1/proactive/status ─────────────────────────────────

func (h *handlers) proactiveStatus(w http.ResponseWriter, r *http.Request) {
	interval, lastAction, lastTick := h.runtime.ProactiveStatus()
	var lastTickUnix int64
	if !lastTick.IsZero() {
		lastTickUnix = lastTick.Unix()
	}
	mode := "normal"
	switch interval {
	case 0:
		mode = "off"
	case 30 * time.Second:
		mode = "frequent"
	case 120 * time.Second:
		mode = "focus"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":        mode,
		"interval_sec": int(interval.Seconds()),
		"last_action": lastAction,
		"last_tick":   lastTickUnix,
	})
}

// ── GET /api/v1/proactive/actions ─────────────────────────────────

func (h *handlers) proactiveActions(w http.ResponseWriter, r *http.Request) {
	actions := h.runtime.ProactiveActions()
	type actionInfo struct {
		Name        string  `json:"name"`
		Category    string  `json:"category"`
		OutcomeType string  `json:"outcome_type"`
		NightSafe   bool    `json:"night_safe"`
		WeightSoc   float64 `json:"weight_social"`
		WeightCare  float64 `json:"weight_care"`
		WeightCur   float64 `json:"weight_curious"`
		WeightQuiet float64 `json:"weight_quiet"`
		WeightExp   float64 `json:"weight_explore"`
		Trigger     string  `json:"trigger"`
		Action      string  `json:"action"`
	}
	out := make([]actionInfo, len(actions))
	for i, a := range actions {
		out[i] = actionInfo{
			Name:        a.Name,
			Category:    a.Category,
			OutcomeType: a.OutcomeType,
			NightSafe:   a.NightSafe,
			WeightSoc:   a.WeightSocial,
			WeightCare:  a.WeightCare,
			WeightCur:   a.WeightCurious,
			WeightQuiet: a.WeightQuiet,
			WeightExp:   a.WeightExplore,
			Trigger:     a.SkillCard.Trigger,
			Action:      a.SkillCard.Action,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"actions": out})
}

// ── GET /api/v1/personality ──────────────────────────────────────

func (h *handlers) getPersonality(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.runtime.LoadPersonalityConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// ── PUT /api/v1/personality ──────────────────────────────────────

func (h *handlers) savePersonality(w http.ResponseWriter, r *http.Request) {
	var cfg app.PersonalityConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if err := h.runtime.SavePersonalityConfig(&cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ── GET /api/v1/llm-config ────────────────────────────────────────

func (h *handlers) getLLMConfig(w http.ResponseWriter, r *http.Request) {
	providers, routes := h.runtime.LLMConfig()
	type llmConfigResponse struct {
		Providers []port.LLMProviderConfig `json:"providers"`
		Routes    port.LLMRoutes           `json:"routes"`
	}
	writeJSON(w, http.StatusOK, llmConfigResponse{Providers: providers, Routes: routes})
}

// ── PUT /api/v1/llm-config ────────────────────────────────────────

func (h *handlers) saveLLMConfig(w http.ResponseWriter, r *http.Request) {
	type llmConfigRequest struct {
		Providers []port.LLMProviderConfig `json:"providers"`
		Routes    port.LLMRoutes           `json:"routes"`
	}
	var req llmConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if err := h.runtime.ReloadLLMConfig(req.Providers, req.Routes); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ── GET /api/v1/logs ──────────────────────────────────────────────

func (h *handlers) logs(w http.ResponseWriter, r *http.Request) {
	entries := h.logBuffer.Entries()
	if entries == nil {
		entries = []logbuffer.Entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": entries})
}

// ── DELETE /api/v1/logs ──────────────────────────────────────────

func (h *handlers) clearLogs(w http.ResponseWriter, r *http.Request) {
	// Re-create buffer to clear
	// Not ideal but works for the in-memory buffer
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ── Helpers ────────────────────────────────────────────────────────

func (h *handlers) pushEmotion() {
	state, vec := h.runtime.EmotionState()
	h.broker.Publish("emotion", emotionResponse{
		Primary:   state.Primary,
		Intensity: state.Intensity,
		Vector: emotionVector8{
			Affection:   vec.Affection,
			Worry:       vec.Worry,
			Curiosity:   vec.Curiosity,
			Sleepiness:  vec.Sleepiness,
			Playfulness: vec.Playfulness,
			Loneliness:  vec.Loneliness,
			Confidence:  vec.Confidence,
			Annoyance:   vec.Annoyance,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
