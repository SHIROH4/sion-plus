// Package http implements the REST API transport layer.
//
// Design rules:
//   1. Handler files ONLY do: parse request → validate → call service → serialize response
//   2. No business logic in handlers — all logic lives in app/modules
//   3. No direct DB access — all data access goes through port interfaces
//
// Files:
//   server.go     — HTTP server setup
//   middleware.go — CORS, request logging, ready check
//   handlers.go   — API handlers

package http
