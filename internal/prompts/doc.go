// Package prompts contains all LLM prompt templates used by the project.
//
// Design rules:
//   1. Every prompt is a Go const string in its own file, organized by domain.
//   2. Prompts are PURE TEXT — no imports, no logic, no parameter substitution here.
//      Callers use fmt.Sprintf to inject dynamic content.
//   3. NEVER hardcode a prompt string in adapter/ or app/ code.
//      Always reference a const from this package.
//   4. Each file has ONE const per language if i18n is needed.
//   5. Files are named by their domain: emotion.go, memory.go, system.go, etc.
//
// Adding a new prompt:
//   1. Find the right file (or create one).
//   2. Add a const with a descriptive name: PromptFactExtraction, PromptSignalDetection...
//   3. Use backticks for multi-line prompts.
//   4. Keep prompts under ~3000 chars each for readability.
package prompts
