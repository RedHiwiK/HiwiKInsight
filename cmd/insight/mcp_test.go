package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testToken = "test-token"

// fakeAPI imitates the read-only query API closely enough for the MCP server.
func fakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"error":"unauthorized"}`)
			return
		}
		switch {
		case r.URL.Path == "/v1/query/describe":
			w.Header().Set("Content-Type", "text/markdown")
			io.WriteString(w, "# Semantic layer\n")
		case r.URL.Path == "/v1/query/meta":
			io.WriteString(w, `{"apps":[{"key":"myapp","name":"My App","bundle":"com.example.myapp"}],"today":"2026-10-09","timezone":"UTC","currency":"USD"}`)
		case r.URL.Path == "/v1/query/metric/retention":
			q := r.URL.Query()
			resp := map[string]any{
				"metric": "retention", "definition": "truly new users",
				"app": q.Get("app"), "env": q.Get("env"), "from": q.Get("from"), "to": q.Get("to"),
				"tables": []any{map[string]any{"name": "cohorts", "columns": []string{"cohort", "event"},
					"rows": [][]any{{"2026-09-01", q.Get("event")}}}},
			}
			json.NewEncoder(w).Encode(resp)
		case strings.HasPrefix(r.URL.Path, "/v1/query/metric/"):
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error":"unknown metric"}`)
		case r.URL.Path == "/v1/query/sql" && r.Method == http.MethodPost:
			var p struct{ SQL string }
			json.NewDecoder(r.Body).Decode(&p)
			if !strings.HasPrefix(strings.ToUpper(p.SQL), "SELECT") {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, `{"error":"only SELECT is allowed"}`)
				return
			}
			io.WriteString(w, `{"columns":["n"],"rows":[[1]],"truncated":false}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error":"not found"}`)
		}
	}))
}

func testClient(endpoint, token string) *client {
	return &client{endpoint: endpoint, token: token, tokenSource: "test", http: &http.Client{Timeout: 5 * time.Second}}
}

// roundTrip feeds the given lines to a fresh server and returns the decoded responses.
func roundTrip(t *testing.T, c *client, lines ...string) []map[string]any {
	t.Helper()
	var out, logs bytes.Buffer
	srv := &mcpServer{client: c, log: &logs}
	if err := srv.serve(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	var resps []map[string]any
	dec := json.NewDecoder(&out)
	for dec.More() {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("decode response: %v\n%s", err, out.String())
		}
		resps = append(resps, m)
	}
	return resps
}

func toolText(t *testing.T, resp map[string]any) (string, bool) {
	t.Helper()
	res, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result in %v", resp)
	}
	content := res["content"].([]any)
	first := content[0].(map[string]any)
	if first["type"] != "text" {
		t.Fatalf("content type = %v", first["type"])
	}
	return first["text"].(string), res["isError"].(bool)
}

func TestMCP(t *testing.T) {
	api := fakeAPI(t)
	defer api.Close()

	tests := []struct {
		name   string
		token  string
		lines  []string
		nResp  int
		verify func(t *testing.T, resps []map[string]any)
	}{
		{
			name:  "initialize negotiates latest version",
			lines: []string{`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`},
			nResp: 1,
			verify: func(t *testing.T, r []map[string]any) {
				res := r[0]["result"].(map[string]any)
				if res["protocolVersion"] != "2025-06-18" {
					t.Errorf("protocolVersion = %v", res["protocolVersion"])
				}
				if res["serverInfo"].(map[string]any)["name"] != "hiwikinsight" {
					t.Errorf("serverInfo = %v", res["serverInfo"])
				}
				if _, ok := res["capabilities"].(map[string]any)["tools"]; !ok {
					t.Errorf("capabilities missing tools: %v", res["capabilities"])
				}
				if !strings.Contains(res["instructions"].(string), "describe") {
					t.Errorf("instructions should mention describe")
				}
			},
		},
		{
			name:  "initialize accepts older version",
			lines: []string{`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`},
			nResp: 1,
			verify: func(t *testing.T, r []map[string]any) {
				if v := r[0]["result"].(map[string]any)["protocolVersion"]; v != "2024-11-05" {
					t.Errorf("protocolVersion = %v", v)
				}
			},
		},
		{
			name:  "initialize unknown version falls back",
			lines: []string{`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`},
			nResp: 1,
			verify: func(t *testing.T, r []map[string]any) {
				if v := r[0]["result"].(map[string]any)["protocolVersion"]; v != mcpProtocolVersion {
					t.Errorf("protocolVersion = %v", v)
				}
			},
		},
		{
			name: "initialized notification and ping",
			lines: []string{
				`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
				`{"jsonrpc":"2.0","id":"p1","method":"ping"}`,
			},
			nResp: 1,
			verify: func(t *testing.T, r []map[string]any) {
				if r[0]["id"] != "p1" || r[0]["error"] != nil {
					t.Errorf("ping response = %v", r[0])
				}
			},
		},
		{
			name:  "tools/list",
			lines: []string{`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`},
			nResp: 1,
			verify: func(t *testing.T, r []map[string]any) {
				tools := r[0]["result"].(map[string]any)["tools"].([]any)
				got := map[string]map[string]any{}
				for _, x := range tools {
					tool := x.(map[string]any)
					got[tool["name"].(string)] = tool
				}
				for _, want := range []string{"describe", "list_apps", "metric", "sql", "user"} {
					tool, ok := got[want]
					if !ok {
						t.Errorf("missing tool %s", want)
						continue
					}
					if tool["inputSchema"].(map[string]any)["type"] != "object" {
						t.Errorf("tool %s inputSchema not an object", want)
					}
				}
				req := got["metric"]["inputSchema"].(map[string]any)["required"].([]any)
				if len(req) != 1 || req[0] != "name" {
					t.Errorf("metric required = %v", req)
				}
			},
		},
		{
			name:  "metric call passes params",
			lines: []string{`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"metric","arguments":{"name":"retention","app":"myapp","env":"production","from":"2026-09-01","to":"2026-09-30","args":{"event":"entry.created"}}}}`},
			nResp: 1,
			verify: func(t *testing.T, r []map[string]any) {
				text, isErr := toolText(t, r[0])
				if isErr {
					t.Fatalf("unexpected error: %s", text)
				}
				var res struct {
					Metric, App, Env, From, To string
					Tables                     []table
				}
				if err := json.Unmarshal([]byte(text), &res); err != nil {
					t.Fatalf("result not JSON: %v", err)
				}
				if res.Metric != "retention" || res.App != "myapp" || res.Env != "production" || res.From != "2026-09-01" || res.To != "2026-09-30" {
					t.Errorf("result = %+v", res)
				}
				if res.Tables[0].Rows[0][1] != "entry.created" {
					t.Errorf("extra arg not forwarded: %v", res.Tables[0].Rows)
				}
			},
		},
		{
			name: "describe and list_apps",
			lines: []string{
				`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"describe","arguments":{}}}`,
				`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"list_apps"}}`,
			},
			nResp: 2,
			verify: func(t *testing.T, r []map[string]any) {
				if text, isErr := toolText(t, r[0]); isErr || !strings.HasPrefix(text, "# Semantic layer") {
					t.Errorf("describe = %q (isError %v)", text, isErr)
				}
				if text, isErr := toolText(t, r[1]); isErr || !strings.Contains(text, `"myapp"`) {
					t.Errorf("list_apps = %q (isError %v)", text, isErr)
				}
			},
		},
		{
			name:  "sql API error becomes isError result",
			lines: []string{`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"sql","arguments":{"sql":"DELETE FROM events"}}}`},
			nResp: 1,
			verify: func(t *testing.T, r []map[string]any) {
				if r[0]["error"] != nil {
					t.Fatalf("API error must not be a JSON-RPC error: %v", r[0])
				}
				text, isErr := toolText(t, r[0])
				if !isErr || !strings.Contains(text, "only SELECT is allowed") {
					t.Errorf("got %q isError=%v", text, isErr)
				}
			},
		},
		{
			name:  "unauthorized explains how to set token",
			token: "wrong",
			lines: []string{`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"describe"}}`},
			nResp: 1,
			verify: func(t *testing.T, r []map[string]any) {
				text, isErr := toolText(t, r[0])
				if !isErr || !strings.Contains(text, "INSIGHT_TOKEN") {
					t.Errorf("got %q isError=%v", text, isErr)
				}
			},
		},
		{
			name:  "missing required argument",
			lines: []string{`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"user","arguments":{}}}`},
			nResp: 1,
			verify: func(t *testing.T, r []map[string]any) {
				if _, isErr := toolText(t, r[0]); !isErr {
					t.Errorf("expected isError")
				}
			},
		},
		{
			name:  "unknown method",
			lines: []string{`{"jsonrpc":"2.0","id":9,"method":"resources/list"}`},
			nResp: 1,
			verify: func(t *testing.T, r []map[string]any) {
				e, ok := r[0]["error"].(map[string]any)
				if !ok || e["code"].(float64) != -32601 {
					t.Errorf("response = %v", r[0])
				}
				if r[0]["id"].(float64) != 9 {
					t.Errorf("id = %v", r[0]["id"])
				}
			},
		},
		{
			name:  "unknown tool",
			lines: []string{`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"drop_tables"}}`},
			nResp: 1,
			verify: func(t *testing.T, r []map[string]any) {
				e, ok := r[0]["error"].(map[string]any)
				if !ok || e["code"].(float64) != -32602 {
					t.Errorf("response = %v", r[0])
				}
			},
		},
		{
			name:  "parse error",
			lines: []string{`{not json`},
			nResp: 1,
			verify: func(t *testing.T, r []map[string]any) {
				e, ok := r[0]["error"].(map[string]any)
				if !ok || e["code"].(float64) != -32700 {
					t.Errorf("response = %v", r[0])
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			token := tc.token
			if token == "" {
				token = testToken
			}
			resps := roundTrip(t, testClient(api.URL, token), tc.lines...)
			if len(resps) != tc.nResp {
				t.Fatalf("got %d responses, want %d: %v", len(resps), tc.nResp, resps)
			}
			for _, r := range resps {
				if r["jsonrpc"] != "2.0" {
					t.Errorf("jsonrpc = %v", r["jsonrpc"])
				}
			}
			tc.verify(t, resps)
		})
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		in        []string
		wantArgs  []string
		wantFlags map[string]string
	}{
		{[]string{"metric", "retention", "--app", "myapp", "--json", "--from=2026-09-01"},
			[]string{"metric", "retention"}, map[string]string{"app": "myapp", "json": "", "from": "2026-09-01"}},
		{[]string{"--json", "sql", "--", "-- comment\nSELECT 1"},
			[]string{"sql", "-- comment\nSELECT 1"}, map[string]string{"json": ""}},
	}
	for _, tc := range tests {
		args, flags := parseArgs(tc.in)
		if strings.Join(args, "|") != strings.Join(tc.wantArgs, "|") {
			t.Errorf("parseArgs(%q) args = %q, want %q", tc.in, args, tc.wantArgs)
		}
		if len(flags) != len(tc.wantFlags) {
			t.Errorf("parseArgs(%q) flags = %v, want %v", tc.in, flags, tc.wantFlags)
		}
		for k, v := range tc.wantFlags {
			if flags[k] != v {
				t.Errorf("parseArgs(%q) flag %s = %q, want %q", tc.in, k, flags[k], v)
			}
		}
	}
}
