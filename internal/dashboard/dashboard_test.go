package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestStatic(t *testing.T) {
	dist := fstest.MapFS{
		"dist/app/index.html":      {Data: []byte("<html>app</html>")},
		"dist/app/assets/app-1.js": {Data: []byte("js")},
		"dist/app/favicon.svg":     {Data: []byte("<svg/>")},
	}
	mux := http.NewServeMux()
	New(dist).Register(mux)
	for path, want := range map[string]string{
		"/dashboard/":                "<html>app</html>",
		"/dashboard/users/abc":       "<html>app</html>", // client-side route fallback
		"/dashboard/assets/app-1.js": "js",
		"/dashboard/favicon.svg":     "<svg/>",
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Body.String() != want {
			t.Errorf("%s = %d %q", path, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/dashboard/assets/missing.js", nil))
	if w.Code != 404 {
		t.Errorf("missing asset = %d", w.Code)
	}
}

func TestNotBuilt(t *testing.T) {
	mux := http.NewServeMux()
	New(fstest.MapFS{}).Register(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/dashboard/", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("unbuilt dashboard = %d", w.Code)
	}
}
