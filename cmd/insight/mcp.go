package main

// A minimal Model Context Protocol server over stdio (newline-delimited
// JSON-RPC 2.0). It exposes the read-only query API as tools so any MCP client
// (Claude Desktop, Claude Code, IDE assistants, ...) can analyse app data.
//
// stdout is the protocol channel: everything else is logged to stderr.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

const mcpProtocolVersion = "2025-06-18"

// Older protocol versions we can speak; if the client asks for one we echo it.
var mcpSupportedVersions = map[string]bool{
	"2025-06-18": true,
	"2025-03-26": true,
	"2024-11-05": true,
}

const mcpInstructions = `HiwiKInsight is a read-only analytics and App Store revenue store for indie iOS apps.
Always call the describe tool first in a session: it documents tables, views, conventions (reporting time zone, environments, what counts as a new user), event catalogs and every predefined metric. Do not assume column or event names without it.
Prefer the metric tool (definitions are fixed and documented); use sql only when no metric fits. Use list_apps to find app keys.
Default to env=production unless asked otherwise. Samples are often small: always report absolute counts alongside ratios, include the date range and env, and do not draw conclusions from single-digit samples. Only state what the data supports.`

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError"`
}

type mcpServer struct {
	client *client
	log    io.Writer
}

func readOnly() map[string]any {
	return map[string]any{"readOnlyHint": true, "openWorldHint": false}
}

func objectSchema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

var mcpTools = []mcpTool{
	{
		Name:  "describe",
		Title: "Describe the data",
		Description: "Return the semantic layer as Markdown: tables and views with their columns, conventions " +
			"(reporting time zone, environments, anonymous install_id, what counts as a truly new user), event catalogs, " +
			"current data volume and the list of predefined metrics with their definitions and parameters. " +
			"Call this first before any other tool.",
		InputSchema: objectSchema(map[string]any{}),
		Annotations: readOnly(),
	},
	{
		Name:        "list_apps",
		Title:       "List apps",
		Description: "List the apps tracked by the server (key, name, bundle id, ...) plus today's date, reporting time zone and currency. Use the app key as the `app` argument of other tools.",
		InputSchema: objectSchema(map[string]any{}),
		Annotations: readOnly(),
	},
	{
		Name:  "metric",
		Title: "Run a predefined metric",
		Description: "Run a predefined metric with a fixed, documented definition and return JSON " +
			"{metric, definition, app, env, from, to, tables:[{name, columns, rows}]}. " +
			"Metrics include active_users, activity_days, retention, returning, module_usage, funnel, revenue, " +
			"distribution, overview, sessions_daily, users, event_trend, errors, portfolio, store_funnel; " +
			"see describe for the full list and each metric's extra parameters.",
		InputSchema: objectSchema(map[string]any{
			"name": strProp("Metric name, e.g. retention or active_users."),
			"app":  strProp("App key from list_apps. Omit to use the server's default app."),
			"env":  strProp("Environment, e.g. production (default), sandbox, xcode."),
			"from": strProp("Start date YYYY-MM-DD in the reporting time zone (inclusive)."),
			"to":   strProp("End date YYYY-MM-DD in the reporting time zone (inclusive)."),
			"args": map[string]any{
				"type":                 "object",
				"description":          "Extra metric-specific parameters as string values, e.g. {\"event\": \"entry.created\"}.",
				"additionalProperties": map[string]any{"type": "string"},
			},
		}, "name"),
		Annotations: readOnly(),
	},
	{
		Name:  "sql",
		Title: "Run read-only SQL",
		Description: "Run one read-only SQLite query (a single SELECT or WITH statement; max 1000 rows; 10 second timeout) " +
			"and return JSON {columns, rows, truncated}. Prefer the views v_user_days, v_user_summary, v_sessions and " +
			"v_screen_usage; use local_today() and local_date(ms) for dates in the reporting time zone. " +
			"Use only when no predefined metric answers the question.",
		InputSchema: objectSchema(map[string]any{
			"sql": strProp("The SELECT/WITH statement to run."),
		}, "sql"),
		Annotations: readOnly(),
	},
	{
		Name:        "user",
		Title:       "Inspect one user",
		Description: "Return one anonymous user's summary, activity calendar and most recent events as JSON (same shape as metric). Users are identified by install_id.",
		InputSchema: objectSchema(map[string]any{
			"install_id": strProp("The anonymous install_id."),
			"app":        strProp("App key. Omit to use the server's default app."),
			"limit":      map[string]any{"type": "integer", "description": "Maximum number of recent events to return.", "minimum": 1},
		}, "install_id"),
		Annotations: readOnly(),
	},
}

func (s *mcpServer) logf(format string, a ...any) {
	if s.log != nil {
		fmt.Fprintf(s.log, "insight mcp: "+format+"\n", a...)
	}
}

// serve reads one JSON-RPC message per line until EOF.
func (s *mcpServer) serve(in io.Reader, out io.Writer) error {
	r := bufio.NewReader(in)
	enc := json.NewEncoder(out)
	s.logf("ready (endpoint %s)", s.client.endpoint)
	for {
		line, err := r.ReadBytes('\n')
		if trimmed := bytes.TrimSpace(line); len(trimmed) > 0 {
			if resp := s.handle(trimmed); resp != nil {
				if werr := enc.Encode(resp); werr != nil {
					return werr
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// handle processes one message and returns the response, or nil for notifications.
func (s *mcpServer) handle(line []byte) *rpcResponse {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		s.logf("parse error: %v", err)
		return &rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}}
	}
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"
	if isNotification {
		// notifications/initialized, notifications/cancelled, ...: never answered.
		return nil
	}
	resp := &rpcResponse{JSONRPC: "2.0", ID: req.ID}
	if req.JSONRPC != "2.0" || req.Method == "" {
		resp.Error = &rpcError{-32600, "invalid request"}
		return resp
	}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
			ClientInfo      struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"clientInfo"`
		}
		_ = json.Unmarshal(req.Params, &p)
		v := mcpProtocolVersion
		if mcpSupportedVersions[p.ProtocolVersion] {
			v = p.ProtocolVersion
		}
		s.logf("initialize from %s %s (protocol %s -> %s)", p.ClientInfo.Name, p.ClientInfo.Version, p.ProtocolVersion, v)
		resp.Result = map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "hiwikinsight", "title": "HiwiKInsight", "version": version},
			"instructions":    mcpInstructions,
		}
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = map[string]any{"tools": mcpTools}
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			resp.Error = &rpcError{-32602, "invalid params: " + err.Error()}
			return resp
		}
		if !knownTool(p.Name) {
			resp.Error = &rpcError{-32602, "unknown tool: " + p.Name}
			return resp
		}
		resp.Result = s.callTool(p.Name, p.Arguments)
	default:
		resp.Error = &rpcError{-32601, "method not found: " + req.Method}
	}
	return resp
}

func knownTool(name string) bool {
	for _, t := range mcpTools {
		if t.Name == name {
			return true
		}
	}
	return false
}

// argString reads an argument as a string, accepting numbers and booleans too.
func argString(args map[string]any, key string) string {
	switch v := args[key].(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}

func (s *mcpServer) callTool(name string, args map[string]any) mcpToolResult {
	if args == nil {
		args = map[string]any{}
	}
	var (
		body []byte
		err  error
	)
	switch name {
	case "describe":
		body, err = s.client.describe()
		if err == nil {
			return textResult(string(body), false)
		}
	case "list_apps":
		body, err = s.client.meta()
	case "metric":
		metric := argString(args, "name")
		if metric == "" {
			return textResult("missing required argument: name", true)
		}
		params := map[string]string{}
		if extra, ok := args["args"].(map[string]any); ok {
			keys := make([]string, 0, len(extra))
			for k := range extra {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				params[k] = argString(extra, k)
			}
		}
		for _, k := range []string{"app", "env", "from", "to"} {
			if v := argString(args, k); v != "" {
				params[k] = v
			}
		}
		body, err = s.client.metric(metric, params)
	case "sql":
		q := argString(args, "sql")
		if q == "" {
			return textResult("missing required argument: sql", true)
		}
		body, err = s.client.sql(q)
	case "user":
		id := argString(args, "install_id")
		if id == "" {
			return textResult("missing required argument: install_id", true)
		}
		body, err = s.client.user(id, argString(args, "app"), argString(args, "limit"))
	}
	if err != nil {
		s.logf("tool %s failed: %v", name, err)
		return textResult(err.Error(), true)
	}
	return textResult(string(bytes.TrimSpace(body)), false)
}

func textResult(text string, isError bool) mcpToolResult {
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: text}}, IsError: isError}
}
