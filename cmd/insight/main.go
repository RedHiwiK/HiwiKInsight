// Command insight is a command-line client for the HiwiKInsight read-only
// query API. It is meant for both humans and AI assistants (Claude Code skills,
// or any MCP client via `insight mcp`).
//
//	insight describe
//	insight metric retention --app myapp --env production --from 2026-09-01 --to 2026-09-30
//	insight sql "SELECT module, COUNT(*) FROM v_screen_usage GROUP BY module"
//	insight user <install_id> --limit 200
//	insight apps
//	insight mcp
//
// Output is a pretty table by default; pass --json for the raw JSON response.
//
// Endpoint: --endpoint, else $INSIGHT_ENDPOINT, else http://localhost:8080.
// Token:    --token, else $INSIGHT_TOKEN, else ~/.config/hiwikinsight/token.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// version is overridden at build time with -ldflags "-X main.version=v1.2.3".
var version = "dev"

const defaultEndpoint = "http://localhost:8080"

const usage = `insight - command-line client for the HiwiKInsight query API

Usage:
  insight describe                      Semantic layer: tables, views, conventions, event
                                        catalogs and the metric list. Read this first.
  insight metric <name> [flags]         Run a predefined metric, e.g. active_users, retention,
                                        funnel, revenue, module_usage (see describe for all).
      --app KEY   --env ENV   --from YYYY-MM-DD   --to YYYY-MM-DD
      --<arg> VALUE                     Any other flag is passed to the metric as a parameter.
  insight sql "<SELECT ...>"            Read-only SQL (single SELECT/WITH, max 1000 rows, 10s).
  insight user <install_id> [flags]     One user's summary, activity calendar and recent events.
      --app KEY   --limit N
  insight apps                          List the apps tracked by the server.
  insight mcp                           Run a Model Context Protocol server over stdio.
  insight version                       Print the CLI version.
  insight help                          Show this help.

Global flags:
  --endpoint URL   Server URL (env INSIGHT_ENDPOINT, default http://localhost:8080)
  --token TOKEN    API token (env INSIGHT_TOKEN, or file ~/.config/hiwikinsight/token)
  --json           Print the raw JSON response instead of tables.
  --               Stop flag parsing (e.g. for SQL that starts with "--").

If --app is omitted the server uses its default (first configured) app.
`

// boolFlags never consume the following argument.
var boolFlags = map[string]bool{"json": true, "help": true, "h": true}

// globalFlags are consumed by the CLI and never forwarded as metric parameters.
var globalFlags = map[string]bool{"json": true, "help": true, "h": true, "endpoint": true, "token": true}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	args, flags := parseArgs(argv)
	_, wantHelp := flags["help"]
	_, wantH := flags["h"]
	if len(args) == 0 || wantHelp || wantH || args[0] == "help" {
		fmt.Fprint(stdout, usage)
		if len(args) == 0 && !wantHelp && !wantH {
			return 2
		}
		return 0
	}
	_, asJSON := flags["json"]
	c := newClient(flags["endpoint"], flags["token"])

	fail := func(msg string) int {
		fmt.Fprintln(stderr, "insight:", msg)
		return 1
	}

	var (
		body []byte
		err  error
	)
	switch args[0] {
	case "version":
		fmt.Fprintln(stdout, "insight", version)
		return 0
	case "mcp":
		srv := &mcpServer{client: c, log: stderr}
		if err := srv.serve(stdin, stdout); err != nil {
			return fail(err.Error())
		}
		return 0
	case "describe":
		body, err = c.describe()
		if err != nil {
			return fail(err.Error())
		}
		stdout.Write(body)
		if len(body) > 0 && body[len(body)-1] != '\n' {
			fmt.Fprintln(stdout)
		}
		return 0
	case "apps":
		body, err = c.meta()
		if err == nil && !asJSON {
			printApps(stdout, body)
			return 0
		}
	case "metric":
		if len(args) < 2 {
			return fail("missing metric name; run `insight describe` for the list")
		}
		params := map[string]string{}
		for k, v := range flags {
			if !globalFlags[k] {
				params[k] = v
			}
		}
		body, err = c.metric(args[1], params)
	case "sql":
		if len(args) < 2 {
			return fail(`missing SQL, e.g. insight sql "SELECT 1"`)
		}
		body, err = c.sql(strings.Join(args[1:], " "))
	case "user":
		if len(args) < 2 {
			return fail("missing install_id")
		}
		body, err = c.user(args[1], flags["app"], flags["limit"])
	default:
		fmt.Fprintf(stderr, "insight: unknown command %q\n\n", args[0])
		fmt.Fprint(stderr, usage)
		return 2
	}
	if err != nil {
		return fail(err.Error())
	}
	if asJSON {
		var out bytes.Buffer
		if json.Indent(&out, body, "", "  ") == nil {
			body = out.Bytes()
		}
		stdout.Write(body)
		fmt.Fprintln(stdout)
		return 0
	}
	printTables(stdout, body)
	return 0
}

// parseArgs lets flags appear anywhere: --k v, --k=v; boolean flags take no value.
// "--" ends flag parsing.
func parseArgs(in []string) ([]string, map[string]string) {
	var args []string
	flags := map[string]string{}
	for i := 0; i < len(in); i++ {
		a := in[i]
		if a == "--" {
			args = append(args, in[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			args = append(args, a)
			continue
		}
		k, v, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !hasValue && !boolFlags[k] && i+1 < len(in) {
			v = in[i+1]
			i++
		}
		flags[k] = v
	}
	return args, flags
}

// ---- HTTP client ----

type client struct {
	endpoint    string
	token       string
	tokenSource string // where the token came from, for error messages
	http        *http.Client
}

func newClient(endpointFlag, tokenFlag string) *client {
	endpoint := strings.TrimSpace(endpointFlag)
	if endpoint == "" {
		endpoint = strings.TrimSpace(os.Getenv("INSIGHT_ENDPOINT"))
	}
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	token, src := loadToken(tokenFlag)
	return &client{
		endpoint:    strings.TrimRight(endpoint, "/"),
		token:       token,
		tokenSource: src,
		http:        &http.Client{Timeout: 30 * time.Second},
	}
}

func tokenFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("~", ".config", "hiwikinsight", "token")
	}
	return filepath.Join(home, ".config", "hiwikinsight", "token")
}

func loadToken(flag string) (token, source string) {
	if t := strings.TrimSpace(flag); t != "" {
		return t, "--token flag"
	}
	if t := strings.TrimSpace(os.Getenv("INSIGHT_TOKEN")); t != "" {
		return t, "INSIGHT_TOKEN"
	}
	path := tokenFilePath()
	if b, err := os.ReadFile(path); err == nil {
		if t := strings.TrimSpace(string(b)); t != "" {
			return t, path
		}
	}
	return "", ""
}

const tokenHelp = `Set a token with one of:
  insight --token <token> ...
  export INSIGHT_TOKEN=<token>
  mkdir -p ~/.config/hiwikinsight && echo '<token>' > ~/.config/hiwikinsight/token
Tokens are configured on the server under query.tokens.`

func (c *client) do(method, path string, payload []byte) ([]byte, error) {
	var rd io.Reader
	if payload != nil {
		rd = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, c.endpoint+path, rd)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach %s: %w (set --endpoint or INSIGHT_ENDPOINT)", c.endpoint, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		if c.token == "" {
			return nil, errors.New("HTTP 401: no API token found.\n" + tokenHelp)
		}
		return nil, fmt.Errorf("HTTP 401: the server rejected the token from %s.\n%s", c.tokenSource, tokenHelp)
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &e) == nil && e.Error != "" {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, e.Error)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func (c *client) describe() ([]byte, error) { return c.do("GET", "/v1/query/describe", nil) }

func (c *client) meta() ([]byte, error) { return c.do("GET", "/v1/query/meta", nil) }

func (c *client) metric(name string, params map[string]string) ([]byte, error) {
	q := url.Values{}
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	return c.do("GET", withQuery("/v1/query/metric/"+url.PathEscape(name), q), nil)
}

func (c *client) sql(query string) ([]byte, error) {
	payload, _ := json.Marshal(map[string]string{"sql": query})
	return c.do("POST", "/v1/query/sql", payload)
}

func (c *client) user(installID, app, limit string) ([]byte, error) {
	q := url.Values{}
	if app != "" {
		q.Set("app", app)
	}
	if limit != "" {
		q.Set("limit", limit)
	}
	return c.do("GET", withQuery("/v1/query/user/"+url.PathEscape(installID), q), nil)
}

func withQuery(path string, q url.Values) string {
	if len(q) == 0 {
		return path
	}
	return path + "?" + q.Encode()
}

// ---- rendering ----

type table struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// printTables handles both response shapes: metric/user (metric + tables) and SQL (columns + rows).
func printTables(w io.Writer, body []byte) {
	var res struct {
		Metric     string   `json:"metric"`
		Definition string   `json:"definition"`
		App        string   `json:"app"`
		Env        string   `json:"env"`
		From       string   `json:"from"`
		To         string   `json:"to"`
		Tables     []table  `json:"tables"`
		Columns    []string `json:"columns"`
		Rows       [][]any  `json:"rows"`
		Truncated  bool     `json:"truncated"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		w.Write(body)
		return
	}
	if res.Metric != "" || len(res.Tables) > 0 {
		fmt.Fprintf(w, "# %s", res.Metric)
		if res.From != "" {
			fmt.Fprintf(w, "  app=%s env=%s %s .. %s", res.App, res.Env, res.From, res.To)
		}
		fmt.Fprintln(w)
		if res.Definition != "" {
			fmt.Fprintf(w, "Definition: %s\n", res.Definition)
		}
		for _, t := range res.Tables {
			fmt.Fprintf(w, "\n## %s\n", t.Name)
			printTable(w, t.Columns, t.Rows)
		}
		return
	}
	printTable(w, res.Columns, res.Rows)
	if res.Truncated {
		fmt.Fprintln(w, "(result truncated at 1000 rows)")
	}
}

// printApps renders /v1/query/meta: one row per app, then the server context.
func printApps(w io.Writer, body []byte) {
	var m struct {
		Apps     []map[string]any `json:"apps"`
		Today    string           `json:"today"`
		Timezone string           `json:"timezone"`
		Currency string           `json:"currency"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		w.Write(body)
		return
	}
	// Fixed leading columns, then any other scalar fields in stable order.
	cols := []string{"key", "name", "bundle"}
	seen := map[string]bool{"key": true, "name": true, "bundle": true}
	var extra []string
	for _, a := range m.Apps {
		for k, v := range a {
			switch v.(type) {
			case map[string]any, []any:
				continue
			}
			if !seen[k] {
				seen[k] = true
				extra = append(extra, k)
			}
		}
	}
	sort.Strings(extra)
	cols = append(cols, extra...)
	rows := make([][]any, len(m.Apps))
	for i, a := range m.Apps {
		row := make([]any, len(cols))
		for j, c := range cols {
			row[j] = a[c]
		}
		rows[i] = row
	}
	printTable(w, cols, rows)
	if m.Today != "" || m.Timezone != "" {
		fmt.Fprintf(w, "\ntoday=%s timezone=%s currency=%s\n", m.Today, m.Timezone, m.Currency)
	}
}

func printTable(w io.Writer, cols []string, rows [][]any) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "(no rows)")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(cols, "\t"))
	for _, r := range rows {
		cells := make([]string, len(r))
		for i, v := range r {
			switch x := v.(type) {
			case nil:
				cells[i] = "-"
			case float64:
				if x == float64(int64(x)) {
					cells[i] = fmt.Sprintf("%d", int64(x))
				} else {
					cells[i] = fmt.Sprintf("%g", x)
				}
			case map[string]any, []any:
				b, _ := json.Marshal(x)
				cells[i] = string(b)
			default:
				cells[i] = fmt.Sprint(x)
			}
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	tw.Flush()
}
