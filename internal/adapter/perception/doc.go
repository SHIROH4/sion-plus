// Package perception implements port.ScreenObserver and port.AppClassifier.
//
// Files:
//   screen_darwin.go   — macOS screen observation and capture
//   app_classifier.go  — app name → category mapping (VS Code→work, Steam→play, etc.)
//   activity_state.go  — activity state tracking
//   emotion_source.go  — emotion signal source

package perception

// TODO: Implement OCR
//   - macOS: VNRecognizeTextRequest (built-in, no extra deps)
//   - Fallback: return empty string if OCR unavailable
