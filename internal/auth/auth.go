// Package auth implements dashboard sign-in.
//
// Users and bcrypt password hashes come from the config file. A successful sign-in
// sets an HttpOnly cookie holding "username|expiry|HMAC-SHA256"; the HMAC key is a
// random secret stored in the data directory, so sessions survive restarts and can be
// revoked by deleting that file. Repeated failures from one IP are throttled.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/RedHiwiK/HiwiKInsight/internal/config"
)

const (
	cookieName  = "hiwikinsight_session"
	maxFailures = 10               // per IP within failureWindow
	failureWin  = 15 * time.Minute // throttle window
)

// dummyHash is compared against for unknown usernames so timing does not reveal them.
var dummyHash = func() string {
	b, _ := bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)
	return string(b)
}()

type Auth struct {
	users  map[string]string // username → bcrypt hash
	ttl    time.Duration
	secret []byte
	// Disabled allows every request (dashboard.insecure_no_auth on a loopback address).
	Disabled bool

	mu       sync.Mutex
	failures map[string][]time.Time
	now      func() time.Time
}

// New loads (or creates) the session secret in dataDir.
func New(cfg config.Dashboard, dataDir string) (*Auth, error) {
	a := &Auth{users: map[string]string{}, ttl: cfg.SessionTTL.Duration, Disabled: cfg.InsecureNoAuth,
		failures: map[string][]time.Time{}, now: time.Now}
	for _, u := range cfg.Users {
		a.users[u.Username] = u.PasswordHash
	}
	secret, err := loadSecret(filepath.Join(dataDir, "session.key"))
	if err != nil {
		return nil, err
	}
	a.secret = secret
	return a, nil
}

func loadSecret(path string) ([]byte, error) {
	if b, err := os.ReadFile(path); err == nil {
		if s, err := hex.DecodeString(strings.TrimSpace(string(b))); err == nil && len(s) >= 32 {
			return s, nil
		}
		return nil, fmt.Errorf("%s is not a valid session key; delete it to create a new one", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	s := make([]byte, 32)
	if _, err := rand.Read(s); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(s)+"\n"), 0o600); err != nil {
		return nil, err
	}
	return s, nil
}

// HashPassword returns a bcrypt hash for the config file.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

// Register mounts /v1/auth/login, /v1/auth/logout and /v1/auth/me.
func (a *Auth) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/auth/login", a.login)
	mux.HandleFunc("POST /v1/auth/logout", a.logout)
	mux.HandleFunc("GET /v1/auth/me", a.me)
}

// User returns the signed-in user of a request, or "".
func (a *Auth) User(r *http.Request) string {
	if a.Disabled {
		return "local"
	}
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return ""
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 {
		return ""
	}
	user, expStr, sig := parts[0], parts[1], parts[2]
	if subtle.ConstantTimeCompare([]byte(sig), []byte(a.sign(user, expStr))) != 1 {
		return ""
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || a.now().Unix() > exp {
		return ""
	}
	if _, ok := a.users[user]; !ok { // removed from the config
		return ""
	}
	return user
}

// Allowed reports whether a request carries a valid session.
func (a *Auth) Allowed(r *http.Request) bool { return a.User(r) != "" }

// Require wraps a handler that needs a signed-in user.
func (a *Auth) Require(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.Allowed(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (a *Auth) sign(user, exp string) string {
	m := hmac.New(sha256.New, a.secret)
	m.Write([]byte(user + "|" + exp))
	return hex.EncodeToString(m.Sum(nil))
}

func (a *Auth) login(w http.ResponseWriter, r *http.Request) {
	if a.Disabled {
		writeJSON(w, http.StatusOK, map[string]string{"username": "local"})
		return
	}
	ip := clientIP(r)
	if a.throttled(ip) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many failed sign-in attempts, try again later"})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": `body must be {"username": "...", "password": "..."}`})
		return
	}
	hash, ok := a.users[req.Username]
	if !ok {
		hash = dummyHash // unknown users take as long as wrong passwords
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil || !ok {
		a.fail(ip)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "wrong username or password"})
		return
	}
	exp := strconv.FormatInt(a.now().Add(a.ttl).Unix(), 10)
	value := base64.RawURLEncoding.EncodeToString([]byte(req.Username + "|" + exp + "|" + a.sign(req.Username, exp)))
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: isHTTPS(r), MaxAge: int(a.ttl.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]string{"username": req.Username})
}

func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
		SameSite: http.SameSiteLaxMode, Secure: isHTTPS(r)})
	w.WriteHeader(http.StatusNoContent)
}

func (a *Auth) me(w http.ResponseWriter, r *http.Request) {
	user := a.User(r)
	if user == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized", "users_configured": len(a.users) > 0})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": user, "auth": !a.Disabled})
}

func (a *Auth) throttled(ip string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	cut := a.now().Add(-failureWin)
	recent := a.failures[ip][:0]
	for _, t := range a.failures[ip] {
		if t.After(cut) {
			recent = append(recent, t)
		}
	}
	a.failures[ip] = recent
	return len(recent) >= maxFailures
}

func (a *Auth) fail(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failures[ip] = append(a.failures[ip], a.now())
}

// clientIP trusts X-Real-IP / X-Forwarded-For only from a loopback or private-network peer
// (a reverse proxy on the same host or in the same Docker network).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		if v := r.Header.Get("X-Real-IP"); v != "" {
			return v
		}
		if v := r.Header.Get("X-Forwarded-For"); v != "" {
			return strings.TrimSpace(strings.Split(v, ",")[0])
		}
	}
	return host
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
