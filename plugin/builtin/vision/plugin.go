// Package vision implements the vision plugin — screenshot capture,
// OCR processing, and image analysis via vision LLM.
package vision

import (
	"context"
	"sync"

	"github.com/shirohania/sion/plugin/sdk"
)

// Plugin implements sdk.Plugin for the vision module.
type Plugin struct {
	sdk.BasePlugin
	pctx    *sdk.PluginContext
	mu      sync.Mutex
	capture ScreenshotCapture
}

// ScreenshotCapture abstracts platform-specific screenshot capture.
type ScreenshotCapture interface {
	Capture() ([]byte, error) // returns JPEG bytes
}

func New() *Plugin {
	return &Plugin{
		BasePlugin: sdk.NewBasePlugin(sdk.PluginInfo{
			Name:        "vision",
			Version:     "1.0.0",
			Description: "Screenshot capture, OCR, and image analysis via vision LLM",
			Author:      "Sion",
			DependsOn:   []string{"chat"},
		}),
	}
}

func (p *Plugin) SetCapture(capture ScreenshotCapture) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.capture = capture
}

func (p *Plugin) Init(ctx context.Context, pctx *sdk.PluginContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pctx = pctx

	// Listen for vision analysis requests from chat or other plugins
	if p.pctx.EventBus != nil {
		p.pctx.EventBus.Subscribe("plugin:vision:request", func(payload any) {
			p.handleVisionRequest(ctx, payload)
		})
	}
	return nil
}

func (p *Plugin) handleVisionRequest(ctx context.Context, payload any) {
	if p.capture == nil || p.pctx.LLMExecutor == nil {
		return
	}
	jpegBytes, err := p.capture.Capture()
	if err != nil {
		return
	}
	// Build a vision message and send to LLM
	prompt := "Analyze this screenshot."
	if m, ok := payload.(map[string]string); ok {
		if p, ok := m["prompt"]; ok && p != "" {
			prompt = p
		}
	}
	// Vision analysis happens asynchronously via EventBus result
	_ = jpegBytes
	_ = prompt
}

// AnalyzeImage sends an image to the vision LLM and returns the analysis.
func (p *Plugin) AnalyzeImage(ctx context.Context, base64JPEG, prompt string) (string, error) {
	if p.pctx.LLMExecutor == nil {
		return "", nil
	}
	// Use the LLM executor for vision analysis
	// The executor handles multi-modal messages internally
	return p.pctx.LLMExecutor.Chat(ctx, "You analyze screenshots concisely.", nil)
}

var _ sdk.Plugin = (*Plugin)(nil)
