package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/labstack/echo/v4"
)

func TestSPAHandler_ServesTheWebAppManifestAsManifestJSON(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html>")},
		"manifest.webmanifest": {Data: []byte(`{"name":"Secretli"}`)},
	}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil)
	rec := httptest.NewRecorder()

	if err := spaHandler(fsys)(e.NewContext(req, rec)); err != nil {
		t.Fatalf("serve manifest: %v", err)
	}

	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/manifest+json") {
		t.Fatalf("Content-Type = %q, want application/manifest+json", got)
	}
}
