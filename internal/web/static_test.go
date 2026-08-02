package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewStaticHandler_ServesIndexAppAndStyle(t *testing.T) {
	h := NewStaticHandler()

	for path, want := range map[string]string{
		"/":          "<title>Janus</title>",
		"/app.js":    "connect()",
		"/style.css": "--bg",
	} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != 200 {
			t.Fatalf("GET %s: status = %d, want 200", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("GET %s: body does not contain %q", path, want)
		}
	}
}
