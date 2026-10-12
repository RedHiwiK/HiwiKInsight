package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// An app that passes its own account ID as appAccountToken reuses one token for every purchase. Each
// notification must still resolve to the attempt that led to the purchase.
func TestAttributionWithStableAccountToken(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	const token = "acc0000-0000-0000-0000-000000000001"
	min := func(day string, hour, minute int) int64 {
		return ms(day, hour) + int64(minute)*int64(time.Minute/time.Millisecond)
	}
	attempt := func(app, install, context string, at int64) {
		t.Helper()
		if _, err := s.WriteBatches([]*Batch{{App: app, InstallID: install, Env: "production", Context: map[string]string{},
			Events: []AnalyticsEvent{{ID: fmt.Sprint(app, install, context, at), Name: "purchase.started", TS: at,
				Params: map[string]string{"context": context, "product": "pro", "token": token}}}}}); err != nil {
			t.Fatal(err)
		}
	}
	note := func(id, typ, reason string, purchaseAt, originalAt int64) {
		t.Helper()
		sign := map[string]int{"SUBSCRIBED": 1, "DID_RENEW": 1, "ONE_TIME_CHARGE": 1, "REFUND": -1}[typ]
		if _, err := s.Record(Event{NotificationUUID: id, BundleID: "com.example.pawprint", Environment: "Production",
			NotificationType: typ, RevenueSign: sign, PriceMilli: 18000, SignedAt: purchaseAt + 1000,
			AppAccountToken: token, App: "pawprint", OriginalTransactionID: "orig-1", TransactionReason: reason,
			PurchaseAt: purchaseAt, OriginalPurchaseAt: originalAt}); err != nil {
			t.Fatal(err)
		}
	}
	context := func(id string) string {
		t.Helper()
		a, err := s.FindAttribution(id)
		if err != nil {
			t.Fatal(err)
		}
		if a == nil {
			return "<none>"
		}
		return a.Context
	}

	start := min("2026-09-01", 10, 6)
	attempt("pawprint", "phone", "onboarding", min("2026-09-01", 10, 0))  // cancelled on the phone
	attempt("pawprint", "ipad", "record_limit", min("2026-09-01", 10, 5)) // bought on the iPad a minute later
	attempt("ledgerly", "ipad", "other_app", min("2026-09-01", 10, 5)+1)  // same account token in another app
	note("buy", "SUBSCRIBED", "PURCHASE", start, start)
	if got := context("buy"); got != "record_limit" {
		t.Errorf("first purchase -> %s, want record_limit (latest attempt before the purchase, same app)", got)
	}
	// Only the install that made the attributed attempt is paid, not the phone that tried and cancelled.
	paid := map[string]int{}
	rows, err := s.db.Query(`SELECT install_id, is_paid FROM v_user_summary WHERE app = 'pawprint'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		var p int
		rows.Scan(&id, &p)
		paid[id] = p
	}
	rows.Close()
	if paid["ipad"] != 1 || paid["phone"] != 0 {
		t.Errorf("is_paid = %v, want ipad 1, phone 0", paid)
	}

	// A later attempt that is cancelled must not take over the renewals and refunds of the running subscription.
	attempt("pawprint", "phone", "settings_banner", min("2026-09-20", 9, 0))
	renew := min("2026-10-01", 10, 6)
	note("renew", "DID_RENEW", "RENEWAL", renew, start)
	note("refund", "REFUND", "RENEWAL", renew, start)
	for _, id := range []string{"renew", "refund"} {
		if got := context(id); got != "record_limit" {
			t.Errorf("%s -> %s, want record_limit (anchored to the purchase that started the subscription)", id, got)
		}
	}

	// Resubscribing keeps the originalTransactionId and originalPurchaseDate but is a new purchase with its own attempt.
	resub := min("2026-12-01", 20, 0)
	attempt("pawprint", "phone", "winback", resub-int64(time.Minute/time.Millisecond))
	note("resub", "SUBSCRIBED", "PURCHASE", resub, start)
	note("resub-renew", "DID_RENEW", "RENEWAL", min("2027-01-01", 20, 0), start)
	for _, id := range []string{"resub", "resub-renew"} {
		if got := context(id); got != "winback" {
			t.Errorf("%s -> %s, want winback", id, got)
		}
	}

	// Clock skew: an attempt stamped up to 5 minutes after the purchase still counts, later ones do not.
	late := min("2027-02-01", 8, 0)
	attempt("pawprint", "phone", "skew_ok", late+4*int64(time.Minute/time.Millisecond))
	note("skew", "ONE_TIME_CHARGE", "PURCHASE", late, late)
	if got := context("skew"); got != "skew_ok" {
		t.Errorf("skew -> %s, want skew_ok", got)
	}
	attempt("pawprint", "phone", "too_late", min("2027-03-01", 8, 6))
	note("toolate", "ONE_TIME_CHARGE", "PURCHASE", min("2027-03-01", 8, 0), min("2027-03-01", 8, 0))
	if got := context("toolate"); got != "skew_ok" {
		t.Errorf("attempt 6 minutes after the purchase -> %s, want the previous attempt skew_ok", got)
	}

	// Seven distinct attempts share the token. The same attempt arriving again under another event ID
	// (token + install + time unchanged) is not stored twice.
	if _, err := s.WriteBatches([]*Batch{{App: "pawprint", InstallID: "phone", Env: "production", Context: map[string]string{},
		Events: []AnalyticsEvent{{ID: "retry", Name: "purchase.started", TS: resub - int64(time.Minute/time.Millisecond),
			Params: map[string]string{"context": "winback", "product": "pro", "token": token}}}}}); err != nil {
		t.Fatal(err)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM purchase_attempts WHERE token = ?`, token).Scan(&n)
	if n != 7 {
		t.Errorf("purchase_attempts for the token = %d, want 7", n)
	}
}

// Tokens generated per purchase keep today's behavior, including rows stored before v11 without purchase times.
func TestAttributionWithPerPurchaseToken(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.WriteBatches([]*Batch{{App: "pawprint", InstallID: "u1", Env: "production", Context: map[string]string{},
		Events: []AnalyticsEvent{{ID: "p1", Name: "purchase.started", TS: ms("2026-09-20", 12),
			Params: map[string]string{"context": "record_limit", "token": "tok-1"}}}}}); err != nil {
		t.Fatal(err)
	}
	// Legacy row: no app, no purchase times, signed before the (clock-skewed) attempt.
	if _, err := s.Record(Event{NotificationUUID: "legacy", BundleID: "com.example.pawprint", Environment: "Production",
		NotificationType: "ONE_TIME_CHARGE", RevenueSign: 1, PriceMilli: 18000, SignedAt: ms("2026-09-20", 11),
		AppAccountToken: "TOK-1"}); err != nil {
		t.Fatal(err)
	}
	// Renewal of a subscription bought before the server existed: no stored PURCHASE, original purchase predates
	// the attempt, so it falls back to the latest attempt before the renewal itself.
	if _, err := s.Record(Event{NotificationUUID: "renew", BundleID: "com.example.pawprint", Environment: "Production",
		NotificationType: "DID_RENEW", RevenueSign: 1, PriceMilli: 18000, SignedAt: ms("2026-10-20", 12),
		AppAccountToken: "tok-1", App: "pawprint", OriginalTransactionID: "orig-9", TransactionReason: "RENEWAL",
		PurchaseAt: ms("2026-10-20", 12), OriginalPurchaseAt: ms("2025-01-01", 12)}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"legacy", "renew"} {
		if a, err := s.FindAttribution(id); err != nil || a == nil || a.Context != "record_limit" {
			t.Errorf("%s -> %+v, %v; want record_limit", id, a, err)
		}
	}
}
