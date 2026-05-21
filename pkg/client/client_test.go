package client_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dsb-labs/torrents/internal/server/api"
	"github.com/dsb-labs/torrents/pkg/client"
)

const testInfoHash = "0123456789abcdef0123456789abcdef01234567"

func TestClient_AddMagnet(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name             string
		Handler          http.HandlerFunc
		Magnet           string
		Label            string
		TargetDir        string
		Expected         client.Torrent
		ExpectConflict   bool
		ExpectBadRequest bool
	}{
		{
			Name: "success",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/api/v1/torrents", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				var got api.AddTorrentRequest
				require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
				assert.Equal(t, "magnet:?xt=urn:btih:"+testInfoHash, got.Magnet)
				assert.Equal(t, "iso", got.Label)

				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(api.AddTorrentResponse{
					Torrent: api.Torrent{InfoHash: testInfoHash, Name: "linux.iso"},
				})
			},
			Magnet:   "magnet:?xt=urn:btih:" + testInfoHash,
			Label:    "iso",
			Expected: client.Torrent{InfoHash: testInfoHash, Name: "linux.iso"},
		},
		{
			Name: "conflict",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(api.ErrorResponse{Message: "torrent already exists"})
			},
			Magnet:         "magnet:?xt=urn:btih:" + testInfoHash,
			ExpectConflict: true,
		},
		{
			Name: "bad request",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(api.ErrorResponse{Message: "magnet: cannot be blank"})
			},
			ExpectBadRequest: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			server := httptest.NewServer(tc.Handler)
			t.Cleanup(server.Close)

			c, err := client.New(server.URL)
			require.NoError(t, err)

			got, err := c.AddMagnet(t.Context(), tc.Magnet, tc.Label, tc.TargetDir)
			switch {
			case tc.ExpectConflict:
				assert.True(t, client.IsConflict(err))
				return
			case tc.ExpectBadRequest:
				assert.True(t, client.IsBadRequest(err))
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.Expected, got)
		})
	}
}

func TestClient_AddFile(t *testing.T) {
	t.Parallel()

	const torrentBytes = "fake .torrent payload"

	tt := []struct {
		Name             string
		Handler          http.HandlerFunc
		File             io.Reader
		Label            string
		TargetDir        string
		Expected         client.Torrent
		ExpectBadRequest bool
	}{
		{
			Name: "success",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/api/v1/torrents", r.URL.Path)

				mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
				require.NoError(t, err)
				assert.Equal(t, "multipart/form-data", mediaType)
				require.NotEmpty(t, params["boundary"])

				require.NoError(t, r.ParseMultipartForm(1<<20))
				file, _, err := r.FormFile("file")
				require.NoError(t, err)
				defer file.Close()

				body, err := io.ReadAll(file)
				require.NoError(t, err)
				assert.Equal(t, torrentBytes, string(body))
				assert.Equal(t, "iso", r.FormValue("label"))
				assert.Equal(t, "/data", r.FormValue("targetDir"))

				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(api.AddTorrentResponse{
					Torrent: api.Torrent{InfoHash: testInfoHash, Name: "linux.iso"},
				})
			},
			File:      strings.NewReader(torrentBytes),
			Label:     "iso",
			TargetDir: "/data",
			Expected:  client.Torrent{InfoHash: testInfoHash, Name: "linux.iso"},
		},
		{
			Name: "bad request",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(api.ErrorResponse{Message: "invalid torrent file"})
			},
			File:             bytes.NewReader([]byte("junk")),
			ExpectBadRequest: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			server := httptest.NewServer(tc.Handler)
			t.Cleanup(server.Close)

			c, err := client.New(server.URL)
			require.NoError(t, err)

			got, err := c.AddFile(t.Context(), tc.File, tc.Label, tc.TargetDir)
			if tc.ExpectBadRequest {
				assert.True(t, client.IsBadRequest(err))
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.Expected, got)
		})
	}
}

func TestClient_List(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name     string
		Handler  http.HandlerFunc
		Expected []client.Torrent
	}{
		{
			Name: "success",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/api/v1/torrents", r.URL.Path)

				_ = json.NewEncoder(w).Encode(api.ListTorrentsResponse{
					Torrents: []api.Torrent{{InfoHash: testInfoHash, Name: "a"}},
				})
			},
			Expected: []client.Torrent{{InfoHash: testInfoHash, Name: "a"}},
		},
		{
			Name: "empty",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(api.ListTorrentsResponse{Torrents: []api.Torrent{}})
			},
			Expected: []client.Torrent{},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			server := httptest.NewServer(tc.Handler)
			t.Cleanup(server.Close)

			c, err := client.New(server.URL)
			require.NoError(t, err)

			got, err := c.List(t.Context())
			require.NoError(t, err)
			assert.Equal(t, tc.Expected, got)
		})
	}
}

func TestClient_Get(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name           string
		Handler        http.HandlerFunc
		Expected       client.Torrent
		ExpectNotFound bool
	}{
		{
			Name: "success",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/api/v1/torrents/"+testInfoHash, r.URL.Path)

				_ = json.NewEncoder(w).Encode(api.GetTorrentResponse{
					Torrent: api.Torrent{InfoHash: testInfoHash, Name: "x.iso"},
				})
			},
			Expected: client.Torrent{InfoHash: testInfoHash, Name: "x.iso"},
		},
		{
			Name: "not found",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(api.ErrorResponse{Message: "does not exist"})
			},
			ExpectNotFound: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			server := httptest.NewServer(tc.Handler)
			t.Cleanup(server.Close)

			c, err := client.New(server.URL)
			require.NoError(t, err)

			got, err := c.Get(t.Context(), testInfoHash)
			if tc.ExpectNotFound {
				assert.True(t, client.IsNotFound(err))
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.Expected, got)
		})
	}
}

func TestClient_Remove(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name           string
		DeleteFiles    bool
		Handler        http.HandlerFunc
		ExpectNotFound bool
	}{
		{
			Name: "success",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodDelete, r.Method)
				assert.Equal(t, "/api/v1/torrents/"+testInfoHash, r.URL.Path)
				assert.Empty(t, r.URL.Query().Get("files"))

				_ = json.NewEncoder(w).Encode(api.RemoveTorrentResponse{})
			},
		},
		{
			Name:        "delete files",
			DeleteFiles: true,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodDelete, r.Method)
				assert.Equal(t, "/api/v1/torrents/"+testInfoHash, r.URL.Path)
				assert.Equal(t, "true", r.URL.Query().Get("files"))

				_ = json.NewEncoder(w).Encode(api.RemoveTorrentResponse{})
			},
		},
		{
			Name: "not found",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(api.ErrorResponse{Message: "does not exist"})
			},
			ExpectNotFound: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			server := httptest.NewServer(tc.Handler)
			t.Cleanup(server.Close)

			c, err := client.New(server.URL)
			require.NoError(t, err)

			err = c.Remove(t.Context(), testInfoHash, tc.DeleteFiles)
			if tc.ExpectNotFound {
				assert.True(t, client.IsNotFound(err))
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestClient_Pause(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name           string
		Handler        http.HandlerFunc
		ExpectNotFound bool
	}{
		{
			Name: "success",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/api/v1/torrents/"+testInfoHash+"/pause", r.URL.Path)

				_ = json.NewEncoder(w).Encode(api.PauseTorrentResponse{})
			},
		},
		{
			Name: "not found",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(api.ErrorResponse{Message: "does not exist"})
			},
			ExpectNotFound: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			server := httptest.NewServer(tc.Handler)
			t.Cleanup(server.Close)

			c, err := client.New(server.URL)
			require.NoError(t, err)

			err = c.Pause(t.Context(), testInfoHash)
			if tc.ExpectNotFound {
				assert.True(t, client.IsNotFound(err))
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestClient_Resume(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name           string
		Handler        http.HandlerFunc
		ExpectNotFound bool
	}{
		{
			Name: "success",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/api/v1/torrents/"+testInfoHash+"/resume", r.URL.Path)

				_ = json.NewEncoder(w).Encode(api.ResumeTorrentResponse{})
			},
		},
		{
			Name: "not found",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(api.ErrorResponse{Message: "does not exist"})
			},
			ExpectNotFound: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			server := httptest.NewServer(tc.Handler)
			t.Cleanup(server.Close)

			c, err := client.New(server.URL)
			require.NoError(t, err)

			err = c.Resume(t.Context(), testInfoHash)
			if tc.ExpectNotFound {
				assert.True(t, client.IsNotFound(err))
				return
			}

			require.NoError(t, err)
		})
	}
}
