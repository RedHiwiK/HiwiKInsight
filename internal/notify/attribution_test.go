package notify

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/store"
)

// The first-purchase notification can arrive before purchase.started is stored;
// attribution must still find it within the wait window.
func TestFindAttributionWaitsForLateEvent(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	h := &Handler{Store: st, AttributionWait: 3 * time.Second}

	go func() {
		time.Sleep(600 * time.Millisecond)
		st.WriteBatches([]*store.Batch{{App: "pawprint", InstallID: "u1", Env: "production", Context: map[string]string{},
			Events: []store.AnalyticsEvent{{ID: "p1", Name: "purchase.started", TS: time.Now().UnixMilli(),
				Params: map[string]string{"context": "pet_limit", "token": "tok-1"}}}}})
	}()
	if a := h.findAttribution("TOK-1", "ONE_TIME_CHARGE"); a == nil || a.Context != "pet_limit" {
		t.Fatalf("attribution = %+v", a)
	}
	// Renewals and other later notifications do not wait
	start := time.Now()
	if a := h.findAttribution("missing", "DID_RENEW"); a != nil || time.Since(start) > 500*time.Millisecond {
		t.Fatalf("renewal lookup = %+v after %v", a, time.Since(start))
	}
}
