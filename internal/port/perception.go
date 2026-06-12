package port

import "context"

// ── Screen Observer ──

// ScreenObserver captures and classifies the user's current screen state.
// Implementation: adapter/perception/screen_darwin.go, screen_windows.go
type ScreenObserver interface {
	Observe(ctx context.Context) (*ScreenObservation, error)
	IsAvailable() bool
}

type ScreenObservation struct {
	AppName     string `json:"app_name"`
	AppCategory string `json:"app_category"` // "work"|"play"|"social"|"idle"
	WindowTitle string `json:"window_title"`
	OCRText     string `json:"ocr_text,omitempty"`
	Screenshot  []byte `json:"screenshot,omitempty"` // compressed JPEG
	Timestamp   int64  `json:"timestamp"`
}

// ── OCR Engine ──

// OCREngine extracts text from images.
// macOS: Vision framework. Windows: Windows.Media.OCR.
type OCREngine interface {
	ExtractText(ctx context.Context, imagePath string) (string, error)
	IsAvailable() bool
}

// ── App Classifier ──

// AppClassifier maps an app name + window title to a semantic category.
type AppClassifier interface {
	Classify(appName, windowTitle string) *AppClassification
}

type AppClassification struct {
	Primary   string `json:"primary"` // "work"|"play"|"social"|"idle"
	Subtype   string `json:"subtype"` // "coding"|"debugging"|"gaming"|"chatting"|"browsing"
	IsWorking bool   `json:"is_working"`
}
