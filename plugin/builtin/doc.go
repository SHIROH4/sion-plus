// Package builtin contains the default plugins shipped with Sion.
//
// Each plugin is an independent package that implements the plugin.Plugin interface.
// Plugins receive their dependencies via PluginContext injection (port interfaces),
// NOT by importing other plugins.
//
// Plugins:
//   chat/   — system prompt builder, OnBeforeChat/OnAfterChat hooks, function tools
//   memory/ — fact extraction, diary generation, memory retrieval, compression
//   vision/ — screenshot OCR, image analysis via vision LLM
//   search/ — web search via Bocha/Bing API
//   qq/     — QQ Bot message relay (WebSocket)
//   timer/  — scheduled reminders (cron-like scheduler → ProactiveIntent submission)

package builtin
