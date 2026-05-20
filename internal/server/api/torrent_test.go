package api_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/dsb-labs/torrents/internal/server/api"
	"github.com/dsb-labs/torrents/internal/server/service"
)

const testInfoHash = "0123456789abcdef0123456789abcdef01234567"

func TestTorrentAPI_Add(t *testing.T) {
	t.Parallel()

	successTorrent := service.Torrent{
		InfoHash:  testInfoHash,
		Magnet:    "magnet:?xt=urn:btih:" + testInfoHash,
		Label:     "iso",
		TargetDir: "/data/downloads",
		CreatedAt: time.Unix(1700000000, 0).UTC(),
		UpdatedAt: time.Unix(1700000000, 0).UTC(),
		Name:      "linux.iso",
		Length:    1024,
	}

	tt := []struct {
		Name           string
		Request        api.AddTorrentRequest
		RawBody        string
		ContentType    string
		Multipart      *multipartBody
		SetupMock      func(*MockTorrentService)
		ExpectedStatus int
		ExpectedBody   func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			Name: "success",
			Request: api.AddTorrentRequest{
				Magnet:    "magnet:?xt=urn:btih:" + testInfoHash,
				Label:     "iso",
				TargetDir: "/data/downloads",
			},
			SetupMock: func(svc *MockTorrentService) {
				svc.EXPECT().AddMagnet(mock.Anything, "magnet:?xt=urn:btih:"+testInfoHash, service.AddOptions{
					Label:     "iso",
					TargetDir: "/data/downloads",
				}).Return(successTorrent, nil).Once()
			},
			ExpectedStatus: http.StatusCreated,
			ExpectedBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := decodeJSON[api.AddTorrentResponse](t, w.Body)
				assert.Equal(t, testInfoHash, body.Torrent.InfoHash)
				assert.Equal(t, "iso", body.Torrent.Label)
				assert.Equal(t, "linux.iso", body.Torrent.Name)
				assert.EqualValues(t, 1024, body.Torrent.Length)
			},
		},
		{
			Name:           "missing magnet",
			Request:        api.AddTorrentRequest{},
			ExpectedStatus: http.StatusBadRequest,
			ExpectedBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := decodeJSON[api.ErrorResponse](t, w.Body)
				assert.Contains(t, body.Message, "magnet")
			},
		},
		{
			Name:           "malformed json",
			RawBody:        "not json",
			ExpectedStatus: http.StatusBadRequest,
		},
		{
			Name:    "duplicate",
			Request: api.AddTorrentRequest{Magnet: "magnet:?xt=urn:btih:" + testInfoHash},
			SetupMock: func(svc *MockTorrentService) {
				svc.EXPECT().AddMagnet(mock.Anything, mock.Anything, mock.Anything).Return(service.Torrent{}, service.ErrTorrentAlreadyExists).Once()
			},
			ExpectedStatus: http.StatusConflict,
			ExpectedBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := decodeJSON[api.ErrorResponse](t, w.Body)
				assert.Contains(t, body.Message, "already exists")
			},
		},
		{
			Name:    "internal error",
			Request: api.AddTorrentRequest{Magnet: "magnet:?xt=urn:btih:" + testInfoHash},
			SetupMock: func(svc *MockTorrentService) {
				svc.EXPECT().AddMagnet(mock.Anything, mock.Anything, mock.Anything).Return(service.Torrent{}, errors.New("boom")).Once()
			},
			ExpectedStatus: http.StatusInternalServerError,
		},
		{
			Name:           "unsupported content type",
			RawBody:        "hello",
			ContentType:    "text/plain",
			ExpectedStatus: http.StatusUnsupportedMediaType,
		},
		{
			Name: "multipart success",
			Multipart: &multipartBody{
				File:   []byte("fake .torrent bytes"),
				Fields: map[string]string{"label": "iso", "targetDir": "/data/downloads"},
			},
			SetupMock: func(svc *MockTorrentService) {
				svc.EXPECT().AddFile(mock.Anything, mock.Anything, service.AddOptions{
					Label:     "iso",
					TargetDir: "/data/downloads",
				}).Return(successTorrent, nil).Once()
			},
			ExpectedStatus: http.StatusCreated,
			ExpectedBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := decodeJSON[api.AddTorrentResponse](t, w.Body)
				assert.Equal(t, testInfoHash, body.Torrent.InfoHash)
			},
		},
		{
			Name:           "multipart missing file",
			Multipart:      &multipartBody{Fields: map[string]string{"label": "iso"}},
			ExpectedStatus: http.StatusBadRequest,
			ExpectedBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := decodeJSON[api.ErrorResponse](t, w.Body)
				assert.Contains(t, body.Message, "file")
			},
		},
		{
			Name:      "multipart invalid file",
			Multipart: &multipartBody{File: []byte("not a torrent")},
			SetupMock: func(svc *MockTorrentService) {
				svc.EXPECT().AddFile(mock.Anything, mock.Anything, mock.Anything).Return(service.Torrent{}, service.ErrInvalidTorrentFile).Once()
			},
			ExpectedStatus: http.StatusBadRequest,
			ExpectedBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := decodeJSON[api.ErrorResponse](t, w.Body)
				assert.Contains(t, body.Message, "invalid torrent file")
			},
		},
		{
			Name:      "multipart duplicate",
			Multipart: &multipartBody{File: []byte("fake .torrent")},
			SetupMock: func(svc *MockTorrentService) {
				svc.EXPECT().AddFile(mock.Anything, mock.Anything, mock.Anything).Return(service.Torrent{}, service.ErrTorrentAlreadyExists).Once()
			},
			ExpectedStatus: http.StatusConflict,
		},
		{
			Name:      "multipart internal error",
			Multipart: &multipartBody{File: []byte("fake .torrent")},
			SetupMock: func(svc *MockTorrentService) {
				svc.EXPECT().AddFile(mock.Anything, mock.Anything, mock.Anything).Return(service.Torrent{}, errors.New("boom")).Once()
			},
			ExpectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			svc := NewMockTorrentService(t)
			if tc.SetupMock != nil {
				tc.SetupMock(svc)
			}

			body, contentType := requestBody(t, tc.Request, tc.RawBody, tc.ContentType, tc.Multipart)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents", body)
			req.Header.Set("Content-Type", contentType)
			w := httptest.NewRecorder()

			api.NewTorrentAPI(svc).Add(w, req)

			assert.Equal(t, tc.ExpectedStatus, w.Code)
			if tc.ExpectedBody != nil {
				tc.ExpectedBody(t, w)
			}
		})
	}
}

type multipartBody struct {
	File   []byte
	Fields map[string]string
}

func TestTorrentAPI_List(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name           string
		SetupMock      func(*MockTorrentService)
		ExpectedStatus int
		ExpectedBody   func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			Name: "success",
			SetupMock: func(svc *MockTorrentService) {
				svc.EXPECT().List(mock.Anything).Return([]service.Torrent{
					{InfoHash: testInfoHash, Name: "a"},
					{InfoHash: "1111111111111111111111111111111111111111", Name: "b"},
				}, nil).Once()
			},
			ExpectedStatus: http.StatusOK,
			ExpectedBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := decodeJSON[api.ListTorrentsResponse](t, w.Body)
				require.Len(t, body.Torrents, 2)
				assert.Equal(t, "a", body.Torrents[0].Name)
				assert.Equal(t, "b", body.Torrents[1].Name)
			},
		},
		{
			Name: "internal error",
			SetupMock: func(svc *MockTorrentService) {
				svc.EXPECT().List(mock.Anything).Return(nil, errors.New("boom")).Once()
			},
			ExpectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			svc := NewMockTorrentService(t)
			if tc.SetupMock != nil {
				tc.SetupMock(svc)
			}

			req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
			w := httptest.NewRecorder()

			api.NewTorrentAPI(svc).List(w, req)

			assert.Equal(t, tc.ExpectedStatus, w.Code)
			if tc.ExpectedBody != nil {
				tc.ExpectedBody(t, w)
			}
		})
	}
}

func TestTorrentAPI_Get(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name           string
		SetupMock      func(*MockTorrentService)
		ExpectedStatus int
		ExpectedBody   func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			Name: "success",
			SetupMock: func(svc *MockTorrentService) {
				svc.EXPECT().Get(mock.Anything, testInfoHash).Return(service.Torrent{
					InfoHash: testInfoHash,
					Name:     "x.iso",
				}, nil).Once()
			},
			ExpectedStatus: http.StatusOK,
			ExpectedBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := decodeJSON[api.GetTorrentResponse](t, w.Body)
				assert.Equal(t, testInfoHash, body.Torrent.InfoHash)
				assert.Equal(t, "x.iso", body.Torrent.Name)
			},
		},
		{
			Name: "not found",
			SetupMock: func(svc *MockTorrentService) {
				svc.EXPECT().Get(mock.Anything, testInfoHash).Return(service.Torrent{}, service.ErrTorrentNotFound).Once()
			},
			ExpectedStatus: http.StatusNotFound,
			ExpectedBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := decodeJSON[api.ErrorResponse](t, w.Body)
				assert.Contains(t, body.Message, "does not exist")
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			svc := NewMockTorrentService(t)
			if tc.SetupMock != nil {
				tc.SetupMock(svc)
			}

			req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents/"+testInfoHash, nil)
			req.SetPathValue("hash", testInfoHash)
			w := httptest.NewRecorder()

			api.NewTorrentAPI(svc).Get(w, req)

			assert.Equal(t, tc.ExpectedStatus, w.Code)
			if tc.ExpectedBody != nil {
				tc.ExpectedBody(t, w)
			}
		})
	}
}

func TestTorrentAPI_Remove(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name           string
		SetupMock      func(*MockTorrentService)
		ExpectedStatus int
		ExpectedBody   func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			Name: "success",
			SetupMock: func(svc *MockTorrentService) {
				svc.EXPECT().Remove(mock.Anything, testInfoHash).Return(nil).Once()
			},
			ExpectedStatus: http.StatusOK,
			ExpectedBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				_ = decodeJSON[api.RemoveTorrentResponse](t, w.Body)
			},
		},
		{
			Name: "not found",
			SetupMock: func(svc *MockTorrentService) {
				svc.EXPECT().Remove(mock.Anything, testInfoHash).Return(service.ErrTorrentNotFound).Once()
			},
			ExpectedStatus: http.StatusNotFound,
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			svc := NewMockTorrentService(t)
			tc.SetupMock(svc)

			req := httptest.NewRequest(http.MethodDelete, "/api/v1/torrents/"+testInfoHash, nil)
			req.SetPathValue("hash", testInfoHash)
			w := httptest.NewRecorder()

			api.NewTorrentAPI(svc).Remove(w, req)

			assert.Equal(t, tc.ExpectedStatus, w.Code)
			if tc.ExpectedBody != nil {
				tc.ExpectedBody(t, w)
			}
		})
	}
}

func TestTorrentAPI_Pause(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name           string
		SetupMock      func(*MockTorrentService)
		ExpectedStatus int
		ExpectedBody   func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			Name: "success",
			SetupMock: func(svc *MockTorrentService) {
				svc.EXPECT().Pause(mock.Anything, testInfoHash).Return(nil).Once()
			},
			ExpectedStatus: http.StatusOK,
			ExpectedBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				_ = decodeJSON[api.PauseTorrentResponse](t, w.Body)
			},
		},
		{
			Name: "not found",
			SetupMock: func(svc *MockTorrentService) {
				svc.EXPECT().Pause(mock.Anything, testInfoHash).Return(service.ErrTorrentNotFound).Once()
			},
			ExpectedStatus: http.StatusNotFound,
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			svc := NewMockTorrentService(t)
			tc.SetupMock(svc)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/"+testInfoHash+"/pause", nil)
			req.SetPathValue("hash", testInfoHash)
			w := httptest.NewRecorder()

			api.NewTorrentAPI(svc).Pause(w, req)

			assert.Equal(t, tc.ExpectedStatus, w.Code)
			if tc.ExpectedBody != nil {
				tc.ExpectedBody(t, w)
			}
		})
	}
}

func TestTorrentAPI_Resume(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name           string
		SetupMock      func(*MockTorrentService)
		ExpectedStatus int
		ExpectedBody   func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			Name: "success",
			SetupMock: func(svc *MockTorrentService) {
				svc.EXPECT().Resume(mock.Anything, testInfoHash).Return(nil).Once()
			},
			ExpectedStatus: http.StatusOK,
			ExpectedBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				_ = decodeJSON[api.ResumeTorrentResponse](t, w.Body)
			},
		},
		{
			Name: "not found",
			SetupMock: func(svc *MockTorrentService) {
				svc.EXPECT().Resume(mock.Anything, testInfoHash).Return(service.ErrTorrentNotFound).Once()
			},
			ExpectedStatus: http.StatusNotFound,
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			svc := NewMockTorrentService(t)
			tc.SetupMock(svc)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/"+testInfoHash+"/resume", nil)
			req.SetPathValue("hash", testInfoHash)
			w := httptest.NewRecorder()

			api.NewTorrentAPI(svc).Resume(w, req)

			assert.Equal(t, tc.ExpectedStatus, w.Code)
			if tc.ExpectedBody != nil {
				tc.ExpectedBody(t, w)
			}
		})
	}
}

func TestRecovery(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /boom", func(w http.ResponseWriter, r *http.Request) {
		panic("kaboom")
	})

	handler := api.Recovery(newTestLogger(t))(mux)

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)

	body := decodeJSON[api.ErrorResponse](t, w.Body)
	assert.Contains(t, body.Message, "internal server error")
}

func requestBody(t *testing.T, request api.AddTorrentRequest, raw, contentType string, mp *multipartBody) (io.Reader, string) {
	t.Helper()

	switch {
	case mp != nil:
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		if mp.File != nil {
			part, err := w.CreateFormFile("file", "test.torrent")
			require.NoError(t, err)
			_, err = part.Write(mp.File)
			require.NoError(t, err)
		}
		for k, v := range mp.Fields {
			require.NoError(t, w.WriteField(k, v))
		}
		require.NoError(t, w.Close())
		return &buf, w.FormDataContentType()
	case raw != "":
		ct := contentType
		if ct == "" {
			ct = "application/json"
		}
		return strings.NewReader(raw), ct
	default:
		var buf bytes.Buffer
		require.NoError(t, json.NewEncoder(&buf).Encode(request))
		return &buf, "application/json"
	}
}

func decodeJSON[T any](t *testing.T, body io.Reader) T {
	t.Helper()

	var v T
	require.NoError(t, json.NewDecoder(body).Decode(&v))
	return v
}

func newTestLogger(t *testing.T) *slog.Logger {
	t.Helper()

	level := slog.LevelError
	if testing.Verbose() {
		level = slog.LevelDebug
	}

	return slog.New(slog.NewTextHandler(t.Output(), &slog.HandlerOptions{
		AddSource: testing.Verbose(),
		Level:     level,
	}))
}
