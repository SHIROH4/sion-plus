package http

import (
	"log"
	"net/http"
	"runtime/debug"
	"time"
)

func withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Panic recovery
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[HTTP] panic: %v\n%s", rec, debug.Stack())
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)

		log.Printf("[HTTP] %s %s %v", r.Method, r.URL.Path, time.Since(start))
	})
}
