// Package ui provides the server-rendered HTML surface of the torrents server.
package ui

import (
	"context"
	"embed"
	"io/fs"
	"net/http"

	"github.com/a-h/templ"
	"github.com/go-playground/form/v4"
)

//go:embed static
var staticFS embed.FS

var formDecoder = form.NewDecoder()

type (
	// View is a function that produces a templ.Component from a view model.
	View[T any] func(model T) templ.Component

	// The Validatable interface describes form types that can validate themselves.
	Validatable interface {
		Validate() error
	}
)

// Static returns an http.Handler that serves the embedded UI assets under /static/.
func Static() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}

	return http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
}

func render[T any](ctx context.Context, w http.ResponseWriter, status int, view View[T], model T) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	if err := view(model).Render(ctx, w); err != nil {
		panic(err)
	}
}

func decode[T Validatable](r *http.Request) (T, error) {
	var t T

	if err := r.ParseForm(); err != nil {
		return t, err
	}

	if err := formDecoder.Decode(&t, r.PostForm); err != nil {
		return t, err
	}

	if err := t.Validate(); err != nil {
		return t, err
	}

	return t, nil
}
