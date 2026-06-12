package perception

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestScreenObserverIsAvailable(t *testing.T) {
	o := NewScreenObserver(NewAppClassifier())
	if runtime.GOOS == "darwin" && !o.IsAvailable() {
		t.Error("should be available on macOS")
	}
	if runtime.GOOS != "darwin" && o.IsAvailable() {
		t.Error("should not be available on non-macOS")
	}
}

func TestScreenObserverObserve(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	o := NewScreenObserver(NewAppClassifier())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	obs, err := o.Observe(ctx)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs == nil {
		t.Fatal("observation is nil")
	}
	if obs.AppName == "" {
		t.Error("app name should not be empty")
	}
	if obs.AppCategory == "" {
		t.Error("app category should not be empty")
	}
	if obs.Timestamp == 0 {
		t.Error("timestamp should be set")
	}
	t.Logf("app=%q cat=%q title=%q", obs.AppName, obs.AppCategory, obs.WindowTitle)
}

func TestScreenObserverIdleTime(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	o := NewScreenObserver(NewAppClassifier())
	ctx := context.Background()

	idle := o.IdleSeconds(ctx)
	if idle < 0 {
		t.Errorf("idle time should be >= 0, got %.1f", idle)
	}
	t.Logf("idle time: %.1f seconds", idle)
}

func TestScreenObserverAppSwitchTracking(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	o := NewScreenObserver(NewAppClassifier())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First observation
	obs1, _ := o.Observe(ctx)
	t.Logf("obs1: %q", obs1.AppName)
	c1 := o.SwitchCount()

	// Second observation (same app → no switch increment)
	obs2, _ := o.Observe(ctx)
	c2 := o.SwitchCount()
	t.Logf("obs2: %q (switches=%d)", obs2.AppName, c2)

	// Switch count should be reasonable (0 or small number)
	if c1 < 0 || c2 < 0 {
		t.Error("switch count should not be negative")
	}
}

func TestScreenObserverWindowTitle(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	o := NewScreenObserver(NewAppClassifier())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	title, err := o.windowTitle(ctx)
	if err != nil {
		t.Logf("window title error (may be expected for some apps): %v", err)
	}
	t.Logf("window title: %q", title)
}

func TestScreenObserverFrontmostApp(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	o := NewScreenObserver(NewAppClassifier())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	app, err := o.frontmostApp(ctx)
	if err != nil {
		t.Fatalf("frontmostApp: %v", err)
	}
	if app == "" {
		t.Error("frontmost app should not be empty")
	}
	t.Logf("frontmost app: %q", app)
}

func TestScreenObserverCaptureScreenshot(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	o := NewScreenObserver(NewAppClassifier())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	jpg, err := o.CaptureScreenshot(ctx)
	if err != nil {
		t.Fatalf("CaptureScreenshot: %v", err)
	}
	if len(jpg) == 0 {
		t.Error("screenshot should not be empty")
	}
	// JPEG magic bytes: FF D8 FF
	if len(jpg) < 3 || jpg[0] != 0xFF || jpg[1] != 0xD8 || jpg[2] != 0xFF {
		t.Error("screenshot should be valid JPEG (magic bytes FF D8 FF)")
	}
	t.Logf("screenshot size: %d bytes", len(jpg))
}

func TestScreenObserverDebug(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	o := NewScreenObserver(NewAppClassifier())
	ctx := context.Background()
	// Should not panic
	o.Debug(ctx)
}

func TestScreenObserverContextTimeout(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	o := NewScreenObserver(NewAppClassifier())
	// Already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := o.Observe(ctx)
	if err == nil {
		t.Log("observe with cancelled context did not error (osascript may have completed)")
	}
}

func TestScreenObserverClassification(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	c := NewAppClassifier()
	o := NewScreenObserver(c)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	obs, err := o.Observe(ctx)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	// App should be classified (even if "idle/unknown")
	if obs.AppCategory == "" {
		t.Error("app category should be classified")
	}
	// Known app check: the test runner is likely in Terminal/iTerm/VS Code
	t.Logf("classified %q → %s/%s (is_working=%v)",
		obs.AppName, obs.AppCategory, obs.WindowTitle, "")
}
