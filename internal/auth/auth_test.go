package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/config"
)

func newAuth(t *testing.T) (*Auth, *http.ServeMux) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(config.Dashboard{Users: []config.User{{Username: "admin", PasswordHash: hash}},
		SessionTTL: config.Duration{Duration: time.Hour}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	a.Register(mux)
	mux.HandleFunc("GET /private", a.Require(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(a.User(r))) }))
	return a, mux
}

func login(mux http.Handler, user, pass string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/auth/login", strings.NewReader(`{"username":"`+user+`","password":"`+pass+`"}`))
	req.RemoteAddr = "203.0.113.7:4000"
	mux.ServeHTTP(rec, req)
	return rec
}

func TestLoginSessionAndExpiry(t *testing.T) {
	a, mux := newAuth(t)
	if rec := login(mux, "admin", "wrong"); rec.Code != 401 {
		t.Fatalf("wrong password: %d", rec.Code)
	}
	if rec := login(mux, "nobody", "correct horse"); rec.Code != 401 {
		t.Fatalf("unknown user: %d", rec.Code)
	}
	rec := login(mux, "admin", "correct horse")
	if rec.Code != 200 {
		t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	cookie := rec.Result().Cookies()[0]
	if !cookie.HttpOnly {
		t.Error("session cookie must be HttpOnly")
	}

	get := func(c *http.Cookie) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/private", nil)
		if c != nil {
			req.AddCookie(c)
		}
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	if get(nil) != 401 || get(cookie) != 200 {
		t.Fatal("session check failed")
	}
	tampered := *cookie
	tampered.Value = strings.ToUpper(cookie.Value[:4]) + cookie.Value[4:]
	if get(&tampered) != 401 {
		t.Error("tampered cookie accepted")
	}
	a.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if get(cookie) != 401 {
		t.Error("expired session accepted")
	}
}

func TestThrottle(t *testing.T) {
	_, mux := newAuth(t)
	for i := 0; i < maxFailures; i++ {
		login(mux, "admin", "wrong")
	}
	if rec := login(mux, "admin", "correct horse"); rec.Code != 429 {
		t.Fatalf("expected throttling, got %d", rec.Code)
	}
}

func TestSecretPersists(t *testing.T) {
	dir := t.TempDir()
	a1, _ := New(config.Dashboard{}, dir)
	a2, _ := New(config.Dashboard{}, dir)
	if string(a1.secret) != string(a2.secret) {
		t.Error("session secret should be reused across restarts")
	}
}
