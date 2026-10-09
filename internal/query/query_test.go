package query

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RedHiwiK/HiwiKInsight/internal/catalog"
	"github.com/RedHiwiK/HiwiKInsight/internal/config"
	"github.com/RedHiwiK/HiwiKInsight/internal/store"
)

const testCatalog = `display_name: Pawprint
modules:
  timeline: Journal timeline
events:
  - name: session.started
    desc: A session started
`

func TestValidateSQL(t *testing.T) {
	ok := []string{
		"SELECT 1",
		"  select * from events;  ",
		"WITH x AS (SELECT 1) SELECT * FROM x",
		"-- comment\nSELECT 1",
	}
	for _, q := range ok {
		if _, err := ValidateSQL(q); err != nil {
			t.Errorf("%q rejected: %v", q, err)
		}
	}
	bad := []string{
		"",
		"INSERT INTO events VALUES (1)",
		"DELETE FROM events",
		"PRAGMA user_version = 3",
		"SELECT 1; DROP TABLE events",
		"ATTACH DATABASE '/tmp/x' AS x",
		"/* x */ UPDATE installs SET env = 'x'",
	}
	for _, q := range bad {
		if _, err := ValidateSQL(q); err == nil {
			t.Errorf("%q accepted", q)
		}
	}
}

func newServer(t *testing.T) (*httptest.Server, *store.Store) {
	s, err := store.Open(filepath.Join(t.TempDir(), "q.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	mux := http.NewServeMux()
	cfg, err := config.Parse([]byte(`
query: {tokens: [secret, second]}
apps:
  - {key: pawprint, bundle_id: com.example.pawprint, name: Pawprint, color: orange}
  - {key: ledger, bundle_id: com.example.ledger}
`))
	if err != nil {
		t.Fatal(err)
	}
	New(s.ReadOnly(), cfg, catalog.FromText(map[string]string{"pawprint": testCatalog})).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, s
}

func do(t *testing.T, method, url, token, body string) (int, string) {
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp.StatusCode, b.String()
}

func TestAuthAndEndpoints(t *testing.T) {
	srv, _ := newServer(t)
	if code, _ := do(t, "GET", srv.URL+"/v1/query/describe", "", ""); code != 401 {
		t.Fatalf("no token: %d", code)
	}
	if code, _ := do(t, "GET", srv.URL+"/v1/query/describe", "wrong", ""); code != 401 {
		t.Fatalf("wrong token: %d", code)
	}
	code, body := do(t, "GET", srv.URL+"/v1/query/describe", "secret", "")
	if code != 200 || !strings.Contains(body, "v_user_summary") || !strings.Contains(body, "session.started") {
		t.Fatalf("describe: %d", code)
	}
	code, body = do(t, "POST", srv.URL+"/v1/query/sql", "secret", `{"sql":"SELECT COUNT(*) AS n FROM device_models"}`)
	if code != 200 || !strings.Contains(body, `"columns":["n"]`) {
		t.Fatalf("sql: %d %s", code, body)
	}
	if code, _ := do(t, "POST", srv.URL+"/v1/query/sql", "secret", `{"sql":"DELETE FROM events"}`); code != 400 {
		t.Fatalf("delete allowed: %d", code)
	}
	code, body = do(t, "GET", srv.URL+"/v1/query/metric/active_users?from=2026-09-01&to=2026-09-03", "secret", "")
	if code != 200 || !strings.Contains(body, `"metric":"active_users"`) {
		t.Fatalf("metric: %d %s", code, body)
	}
	if code, body := do(t, "GET", srv.URL+"/v1/query/meta", "second", ""); code != 200 ||
		!strings.Contains(body, `"key":"pawprint"`) || !strings.Contains(body, `"color":"orange"`) || !strings.Contains(body, `"currency":"USD"`) {
		t.Fatalf("meta: %d %s", code, body)
	}
	if code, _ := do(t, "GET", srv.URL+"/v1/query/metric/nope", "secret", ""); code != 400 {
		t.Fatalf("unknown metric: %d", code)
	}
	if code, _ := do(t, "GET", srv.URL+"/v1/query/metric/active_users?app=evil", "secret", ""); code != 400 {
		t.Fatalf("unknown app: %d", code)
	}
}

// Even when the statement check is bypassed, the read-only connection must reject writes
// (WITH ... INSERT is valid SQLite)
func TestReadOnlyConnectionRejectsWrites(t *testing.T) {
	_, s := newServer(t)
	q, err := ValidateSQL("WITH x AS (SELECT 1) INSERT INTO device_models SELECT 'a', 'b' FROM x")
	if err != nil {
		t.Fatal("expected guard to pass WITH prefix so the connection is tested")
	}
	if _, err := RunSQL(context.Background(), s.ReadOnly(), q); err == nil {
		t.Fatal("write through read-only connection succeeded")
	}
}

func TestAppColorsFillUnusedSlots(t *testing.T) {
	got := AppColors([]config.App{{Key: "a"}, {Key: "b", Color: "purple"}, {Key: "c"}})
	if got["b"] != "purple" || got["a"] != "teal" || got["c"] != "orange" {
		t.Fatalf("colors = %v", got)
	}
}
