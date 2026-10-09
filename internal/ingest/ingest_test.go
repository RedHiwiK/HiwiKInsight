package ingest

import (
	"bytes"
	"compress/flate"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/store"
)

type captureWriter struct {
	batches []*store.Batch
	full    bool
}

func (c *captureWriter) Enqueue(b *store.Batch) bool {
	if c.full {
		return false
	}
	c.batches = append(c.batches, b)
	return true
}

var now = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

func newHandler(cap int) (*Handler, *captureWriter) {
	w := &captureWriter{}
	h := New(map[string]string{"pawprint": "com.example.pawprint"}, w, cap)
	h.now = func() time.Time { return now }
	return h, w
}

func body(t *testing.T, mutate func(m map[string]any)) []byte {
	m := map[string]any{
		"schema": 1, "sdk": "0.1.0", "app": "pawprint", "install_id": "ABC-1",
		"sent_at": now.UnixMilli(),
		"context": map[string]string{"env": "production", "device": "iPhone17,1", "unknown_key": "x"},
		"events": []map[string]any{
			{"id": "E1", "name": "session.started", "ts": now.UnixMilli() - 1000, "session": "S1", "params": map[string]string{"source": "icon"}},
		},
	}
	if mutate != nil {
		mutate(m)
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func post(h *Handler, data []byte, encoding string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(data))
	if encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAcceptsDeflateAndNormalizes(t *testing.T) {
	h, w := newHandler(0)
	var buf bytes.Buffer
	zw, _ := flate.NewWriter(&buf, flate.DefaultCompression)
	zw.Write(body(t, nil))
	zw.Close()

	rec := post(h, buf.Bytes(), "deflate")
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"accepted":1`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	b := w.batches[0]
	if b.InstallID != "abc-1" || b.Env != "production" || b.Events[0].ID != "e1" || b.Events[0].Session != "s1" {
		t.Fatalf("batch not normalized: %+v", b)
	}
	if _, ok := b.Context["unknown_key"]; ok {
		t.Fatal("unknown context key kept")
	}
	// Send time equals receive time, so ts is unchanged
	if b.Events[0].TS != now.UnixMilli()-1000 {
		t.Fatalf("ts = %d", b.Events[0].TS)
	}
}

func TestRejectsBadRequests(t *testing.T) {
	h, _ := newHandler(0)
	cases := map[string]struct {
		data     []byte
		encoding string
		want     int
	}{
		"unknown app":  {body(t, func(m map[string]any) { m["app"] = "evil" }), "", 400},
		"bad schema":   {body(t, func(m map[string]any) { m["schema"] = 2 }), "", 400},
		"no events":    {body(t, func(m map[string]any) { m["events"] = []any{} }), "", 400},
		"no install":   {body(t, func(m map[string]any) { m["install_id"] = "" }), "", 400},
		"invalid json": {[]byte("{"), "", 400},
		"gzip":         {body(t, nil), "gzip", 415},
		"too large":    {bytes.Repeat([]byte("x"), maxBody+1), "", 413},
		"too many events": {body(t, func(m map[string]any) {
			evs := make([]map[string]any, maxEvents+1)
			for i := range evs {
				evs[i] = map[string]any{"id": "x", "name": "a"}
			}
			m["events"] = evs
		}), "", 400},
	}
	for name, c := range cases {
		if rec := post(h, c.data, c.encoding); rec.Code != c.want {
			t.Errorf("%s: status=%d want %d", name, rec.Code, c.want)
		}
	}
}

func TestDropsInvalidEventsAndTruncatesParams(t *testing.T) {
	h, w := newHandler(0)
	params := map[string]string{}
	for i := 0; i < 30; i++ {
		params[strings.Repeat("k", 50)+string(rune('a'+i))] = strings.Repeat("v", 300)
	}
	rec := post(h, body(t, func(m map[string]any) {
		m["events"] = []map[string]any{
			{"id": "ok", "name": "entry.created", "ts": now.UnixMilli(), "params": params},
			{"id": "bad", "name": "Bad Name!", "ts": now.UnixMilli()},
			{"id": "", "name": "entry.created", "ts": now.UnixMilli()},
		}
	}), "")
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"accepted":1`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	p := w.batches[0].Events[0].Params
	if len(p) > maxParams {
		t.Fatalf("params = %d", len(p))
	}
	for k, v := range p {
		if len(k) > maxKeyLen || len(v) > maxValueLen {
			t.Fatalf("param not truncated: %d/%d", len(k), len(v))
		}
	}
}

func TestClockSkewCorrection(t *testing.T) {
	h, w := newHandler(0)
	// Client clock is 1 hour slow: ts is shifted forward by 1 hour after correction
	slow := now.Add(-time.Hour)
	post(h, body(t, func(m map[string]any) {
		m["sent_at"] = slow.UnixMilli()
		m["events"] = []map[string]any{{"id": "a", "name": "x.y", "ts": slow.UnixMilli() - 5000}}
	}), "")
	if got := w.batches[0].Events[0].TS; got != now.UnixMilli()-5000 {
		t.Fatalf("corrected ts = %d, want %d", got, now.UnixMilli()-5000)
	}
	// Skew over 7 days: use the receive time instead
	post(h, body(t, func(m map[string]any) {
		m["sent_at"] = now.AddDate(0, 0, -30).UnixMilli()
		m["events"] = []map[string]any{{"id": "b", "name": "x.y", "ts": 1}}
	}), "")
	if got := w.batches[1].Events[0].TS; got != now.UnixMilli() {
		t.Fatalf("untrusted clock ts = %d", got)
	}
}

func TestDailyCapAndBusy(t *testing.T) {
	h, w := newHandler(2)
	three := func(m map[string]any) {
		m["events"] = []map[string]any{{"id": "1", "name": "a"}, {"id": "2", "name": "a"}, {"id": "3", "name": "a"}}
	}
	if rec := post(h, body(t, three), ""); !strings.Contains(rec.Body.String(), `"accepted":2`) {
		t.Fatalf("cap not applied: %s", rec.Body)
	}
	if rec := post(h, body(t, three), ""); rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"accepted":0`) {
		t.Fatalf("over cap: %d %s", rec.Code, rec.Body)
	}
	h2, w2 := newHandler(0)
	w2.full = true
	if rec := post(h2, body(t, nil), ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("busy status = %d", rec.Code)
	}
	_ = w
}
