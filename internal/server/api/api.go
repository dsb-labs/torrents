// Package api provides the JSON HTTP API surface of the torrents server.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type (
	// The Validatable interface describes request types that can validate themselves.
	Validatable interface {
		Validate() error
	}

	// The ErrorResponse type is the JSON shape returned for error responses.
	ErrorResponse struct {
		Error string `json:"error"`
	}
)

func decode[T Validatable](r io.Reader) (T, error) {
	var t T
	if err := json.NewDecoder(r).Decode(&t); err != nil {
		return t, fmt.Errorf("failed to decode request: %w", err)
	}

	if err := t.Validate(); err != nil {
		return t, err
	}

	return t, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErrorf(w http.ResponseWriter, status int, format string, args ...any) {
	writeJSON(w, status, ErrorResponse{Error: fmt.Sprintf(format, args...)})
}

// Logging returns middleware that records every request's method, path,
// status, and duration at debug level.
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &recordingResponseWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rw, r)

			logger.With(
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.status,
				"duration", time.Since(start),
			).Debug("http request")
		})
	}
}

// Recovery returns middleware that catches panics from downstream handlers,
// logs them, and writes a 500 response.
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rv := recover(); rv != nil {
					logger.With("panic", rv, "path", r.URL.Path).Error("handler panicked")
					writeErrorf(w, http.StatusInternalServerError, "internal server error")
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

type recordingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *recordingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
