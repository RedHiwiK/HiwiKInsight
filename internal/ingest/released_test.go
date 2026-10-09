package ingest

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Batches marked production whose build was never released are recorded as sandbox; nothing changes while the list is not ready
func TestUnreleasedBuildIsSandbox(t *testing.T) {
	cases := []struct {
		build, env string
		known      bool
		want       string
	}{
		{"4", "production", true, "sandbox"},     // 2.4.5 (4) not released
		{"6", "production", true, "production"},  // released
		{"4", "production", false, "production"}, // list not ready
		{"4", "xcode", true, "xcode"},            // only production is changed
	}
	for _, c := range cases {
		w := &captureWriter{}
		h := New(map[string]string{"pawprint": "com.example.pawprint"}, w, 0)
		known := c.known
		h.Released = func(app, version, build string) (bool, bool) {
			return app == "pawprint" && build == "6", known
		}
		body := `{"schema":1,"app":"pawprint","install_id":"u1","context":{"env":"` + c.env + `","app_version":"2.4.5","build":"` + c.build +
			`"},"events":[{"id":"e1","name":"session.started","ts":1}]}`
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/events", strings.NewReader(body)))
		if rec.Code >= 300 || len(w.batches) != 1 {
			t.Fatalf("%+v: status %d, batches %d, body %s", c, rec.Code, len(w.batches), rec.Body.String())
		}
		if got := w.batches[0].Env; got != c.want {
			t.Errorf("%+v: env = %s", c, got)
		}
	}
}
