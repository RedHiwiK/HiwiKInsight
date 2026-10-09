// Package ingest implements POST /v1/events: it receives event batches uploaded by HiwiKInsightKit.
// See PROTOCOL.md in the HiwiKInsightKit repository for the protocol. Client IPs are never read or stored.
package ingest

import (
	"bytes"
	"compress/flate"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/store"
)

const (
	maxBody         = 256 << 10
	maxDecompressed = 2 << 20
	maxEvents       = 200
	maxParams       = 20
	maxKeyLen       = 40
	maxValueLen     = 200
	maxContextLen   = 64
	maxSkew         = 7 * 24 * time.Hour
)

var nameRE = regexp.MustCompile(`^[a-z0-9_.]{1,64}$`)

// Allowlist of batch-level context attributes; unknown fields are dropped.
var contextKeys = []string{
	"app_version", "build", "os", "os_version", "device", "locale", "language",
	"region", "storefront", "env", "appearance", "text_size",
}

var envs = map[string]bool{"production": true, "sandbox": true, "xcode": true}

type enqueuer interface{ Enqueue(*store.Batch) bool }

type Handler struct {
	apps     map[string]string // app key → bundle ID; also serves as the allowlist
	writer   enqueuer
	dailyCap int
	now      func() time.Time
	// Released reports whether an app's "version + build" has been released on the App Store; known = false
	// means the list is not ready yet. When nil, no fallback is applied (App Store Connect not configured).
	Released func(app, version, build string) (released, known bool)

	mu     sync.Mutex
	capDay string
	counts map[string]int // app|install_id → events accepted today (reporting time zone)
}

func New(apps map[string]string, writer enqueuer, dailyCap int) *Handler {
	return &Handler{apps: apps, writer: writer, dailyCap: dailyCap, now: time.Now, counts: map[string]int{}}
}

type request struct {
	Schema    int               `json:"schema"`
	SDK       string            `json:"sdk"`
	App       string            `json:"app"`
	InstallID string            `json:"install_id"`
	SentAt    int64             `json:"sent_at"`
	Context   map[string]string `json:"context"`
	Events    []struct {
		ID      string            `json:"id"`
		Name    string            `json:"name"`
		TS      int64             `json:"ts"`
		Session string            `json:"session"`
		Params  map[string]string `json:"params"`
	} `json:"events"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	received := h.now()
	req, status, err := decode(w, r)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	if req.Schema != 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported schema"})
		return
	}
	if _, ok := h.apps[req.App]; !ok {
		slog.Warn("reject events: unknown app", "app", truncate(req.App, 64))
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown app"})
		return
	}
	installID := strings.ToLower(strings.TrimSpace(req.InstallID))
	if installID == "" || len(installID) > 64 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid install_id"})
		return
	}
	if len(req.Events) == 0 || len(req.Events) > maxEvents {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "events must be 1-200"})
		return
	}

	ctx := map[string]string{}
	for _, k := range contextKeys {
		if v := strings.TrimSpace(req.Context[k]); v != "" {
			ctx[k] = truncate(v, maxContextLen)
		}
	}
	env := ctx["env"]
	if !envs[env] {
		env = "unknown"
	}
	// The SDK falls back to production when AppTransaction is unavailable; a build that was never released can only come from TestFlight
	if env == "production" && h.Released != nil {
		if released, known := h.Released(req.App, ctx["app_version"], ctx["build"]); known && !released {
			env = "sandbox"
		}
	}

	// Clock correction: offset = receive time - client send time (includes network latency, which is negligible)
	offset := received.UnixMilli() - req.SentAt
	trustClock := req.SentAt > 0 && time.Duration(abs(offset))*time.Millisecond <= maxSkew

	b := &store.Batch{App: req.App, InstallID: installID, Env: env, Context: ctx, ReceivedAt: received.UnixMilli()}
	dropped := 0
	for _, e := range req.Events {
		id := strings.ToLower(strings.TrimSpace(e.ID))
		if id == "" || len(id) > 64 || !nameRE.MatchString(e.Name) {
			dropped++
			continue
		}
		ts := received.UnixMilli()
		if trustClock && e.TS > 0 {
			ts = min(e.TS+offset, received.UnixMilli())
		}
		b.Events = append(b.Events, store.AnalyticsEvent{
			ID: id, Name: e.Name, TS: ts, Session: truncate(strings.ToLower(e.Session), 64),
			Params: sanitize(e.Params),
		})
	}

	b.Events = h.applyCap(req.App+"|"+installID, received, b.Events)
	if len(b.Events) > 0 && !h.writer.Enqueue(b) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "busy"})
		return
	}
	if dropped > 0 {
		slog.Warn("dropped invalid events", "app", req.App, "dropped", dropped)
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"accepted": len(b.Events)})
}

// applyCap allows at most dailyCap events per install per day; the excess is silently dropped (still replying 202 so the SDK does not keep retrying).
func (h *Handler) applyCap(key string, now time.Time, events []store.AnalyticsEvent) []store.AnalyticsEvent {
	if h.dailyCap <= 0 {
		return events
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if day := store.Day(now.UnixMilli()); day != h.capDay {
		h.capDay, h.counts = day, map[string]int{}
	}
	left := h.dailyCap - h.counts[key]
	if left <= 0 {
		return nil
	}
	if len(events) > left {
		slog.Warn("install hit daily cap", "key", key)
		events = events[:left]
	}
	h.counts[key] += len(events)
	return events
}

func decode(w http.ResponseWriter, r *http.Request) (*request, int, error) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		return nil, http.StatusRequestEntityTooLarge, errors.New("body too large")
	}
	var reader io.Reader = bytes.NewReader(body)
	switch strings.ToLower(r.Header.Get("Content-Encoding")) {
	case "", "identity":
	case "deflate":
		// The protocol uses raw DEFLATE (the output of Apple's NSData .zlib, without a zlib header)
		reader = flate.NewReader(reader)
	default:
		return nil, http.StatusUnsupportedMediaType, errors.New("unsupported content-encoding")
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxDecompressed+1))
	if err != nil {
		return nil, http.StatusBadRequest, errors.New("invalid compressed body")
	}
	if len(raw) > maxDecompressed {
		return nil, http.StatusRequestEntityTooLarge, errors.New("decompressed body too large")
	}
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, http.StatusBadRequest, errors.New("invalid json")
	}
	return &req, 0, nil
}

// sanitize applies the same limits as the SDK: the first 20 keys in sorted order, keys truncated to 40 and values to 200 characters.
func sanitize(params map[string]string) map[string]string {
	out := map[string]string{}
	keys := make([]string, 0, len(params))
	for k := range params {
		if k != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for i, k := range keys {
		if i >= maxParams {
			break
		}
		out[truncate(k, maxKeyLen)] = truncate(params[k], maxValueLen)
	}
	return out
}

// truncate truncates by character so UTF-8 is never split mid-rune.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
