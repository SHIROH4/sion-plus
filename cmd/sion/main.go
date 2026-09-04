package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/SHIROH4/sion-plus/internal/adapter/config"
	"github.com/SHIROH4/sion-plus/internal/app"
	"github.com/SHIROH4/sion-plus/internal/infra/logbuffer"
	"github.com/SHIROH4/sion-plus/internal/port"
	transporthttp "github.com/SHIROH4/sion-plus/internal/transport/http"
	"github.com/SHIROH4/sion-plus/internal/transport/sse"
)

const defaultPersonality = "你是诗音，一位生活在用户电脑里的 AI 桌面伙伴。回答简洁、自然、真诚；主动建议应尊重用户的专注状态与明确边界。"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dataDir := os.Getenv("SION_DATA_DIR")
	if dataDir == "" {
		dataDir = config.AppDataDir()
	}
	manager := config.NewConfigManager(dataDir)
	appConfig, err := manager.Load()
	if err != nil {
		return err
	}
	applyLLMEnvironment(appConfig)

	runtime, err := app.NewRuntime(app.Config{
		DataDir: dataDir, Personality: defaultPersonality,
		LLMProviders: appConfig.Providers, LLMRoutes: appConfig.Routes, ConfigManager: manager,
		EmbeddingURL:       envOr("SION_EMBED_URL", "http://127.0.0.1:11434"),
		EmbeddingModel:     envOr("SION_EMBED_MODEL", "hf.co/CompendiumLabs/bge-small-zh-v1.5-gguf:latest"),
		EmbeddingDimension: 512,
	})
	if err != nil {
		return err
	}

	ctx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignal()
	if err := runtime.Init(ctx); err != nil {
		return err
	}

	broker := sse.NewBroker()
	broker.Start(ctx)
	defer broker.Stop()
	runtime.CognitionTick.SetBroker(broker)
	if appConfig.Proactive.BaseIntervalSec > 0 {
		runtime.CognitionTick.SetInterval(time.Duration(appConfig.Proactive.BaseIntervalSec) * time.Second)
	}
	if !appConfig.Proactive.ChatEnabled {
		if err := runtime.SetProactiveMode(ctx, "off", 0); err != nil {
			return err
		}
	}
	if err := runtime.Start(ctx); err != nil {
		return err
	}

	logBuffer := logbuffer.New(1000)
	log.SetOutput(io.MultiWriter(os.Stdout, logBuffer.Writer()))
	addr := envOr("SION_ADDR", "127.0.0.1:8080")
	frontendDir := envOr("SION_FRONTEND_DIR", filepath.Join("frontend", "dist"))
	if info, statErr := os.Stat(frontendDir); statErr != nil || !info.IsDir() {
		frontendDir = ""
	}
	server := transporthttp.NewServer(addr, runtime, broker, logBuffer, frontendDir)
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.ListenAndServe() }()
	log.Printf("[Main] Sion ready at http://%s (data=%s)", addr, dataDir)

	select {
	case <-ctx.Done():
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[Main] HTTP shutdown: %v", err)
	}
	return runtime.Stop(shutdownCtx)
}

func applyLLMEnvironment(cfg *port.AppConfig) {
	baseURL := os.Getenv("SION_LLM_URL")
	model := os.Getenv("SION_LLM_MODEL")
	apiKey := os.Getenv("SION_LLM_API_KEY")
	if baseURL == "" && model == "" && apiKey == "" {
		return
	}
	provider := port.LLMProviderConfig{
		Name: "environment", BaseURL: baseURL, APIKey: apiKey, ChatModel: model,
		Enabled: true, Priority: 0, MaxRetries: 1, TimeoutSec: 60,
	}
	if len(cfg.Providers) > 0 {
		provider = cfg.Providers[0]
		provider.Name = "environment"
		provider.Enabled = true
		if baseURL != "" {
			provider.BaseURL = baseURL
		}
		if model != "" {
			provider.ChatModel = model
		}
		if apiKey != "" {
			provider.APIKey = apiKey
		}
	}
	cfg.Providers = []port.LLMProviderConfig{provider}
	cfg.Routes = port.LLMRoutes{Default: provider.Name, Chat: provider.Name,
		Emotion: provider.Name, Memory: provider.Name, Vision: provider.Name,
		Summary: provider.Name, Signal: provider.Name, Search: provider.Name}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
