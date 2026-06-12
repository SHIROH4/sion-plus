// Package perception implements port.ScreenObserver, port.OCREngine, port.AppClassifier.
//
// Files:
//   screen_darwin.go   — macOS: CGWindowListCopyWindowInfo + screencapture
//   screen_windows.go  — Windows: EnumWindows + BitBlt
//   ocr_darwin.go      — macOS: Vision framework OCR
//   ocr_windows.go     — Windows: Windows.Media.OCR
//   app_classifier.go  — app name → category mapping (VS Code→work, Steam→play, etc.)

package perception

// TODO (module 16): Implement platform-specific screen capture
//   - macOS: CGWindowListCopyWindowInfo for active window name
//   - Windows: GetForegroundWindow + GetWindowText
//   - Screenshot: compress to 720p JPEG for LLM vision

// TODO (module 16): Implement OCR
//   - macOS: VNRecognizeTextRequest (built-in, no extra deps)
//   - Windows: Windows.Media.OCR (built-in Windows 10+)
//   - Fallback: return empty string if OCR unavailable

// TODO (module 16): Implement AppClassifier
//   - Hardcoded map for common apps (50+ entries)
//   - Unknown apps → classify by window title keywords
//   - Optional LLM path for ambiguous cases
