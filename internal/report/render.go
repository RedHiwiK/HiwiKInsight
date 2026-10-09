package report

import (
	"bytes"
	"html/template"
	"strings"

	"github.com/RedHiwiK/HiwiKInsight/internal/i18n"
	"github.com/RedHiwiK/HiwiKInsight/internal/mailer"
)

// Text renders the plain-text version (the multipart/alternative fallback).
func (v *View) Text() string {
	var b strings.Builder
	b.WriteString(v.Kicker + " · " + v.Title + "\n\n")
	colon := i18n.T(": ", "：")
	b.WriteString(v.Hero.Label + colon + cellText(v.Hero.Cell) + "\n")
	if v.HeroNote != "" {
		b.WriteString(v.HeroNote + "\n")
	}
	for _, s := range v.Stats {
		b.WriteString(s.Label + colon + cellText(s.Cell) + "\n")
	}
	for _, t := range v.Tables {
		b.WriteString(i18n.T("\n["+t.Title+"]\n", "\n【"+t.Title+"】\n"))
		if t.Note != "" {
			b.WriteString(t.Note + "\n")
		}
		for _, r := range t.Rows {
			parts := make([]string, len(r))
			for i, c := range r {
				if i > 0 && i < len(t.Columns) {
					parts[i] = t.Columns[i] + " " + cellText(c)
				} else {
					parts[i] = cellText(c)
				}
			}
			b.WriteString(strings.Join(parts, "  ") + "\n")
		}
	}
	return b.String()
}

func cellText(c Cell) string {
	if c.Delta == "" {
		return c.Value
	}
	return c.Value + paren(c.Delta)
}

// HTML renders the email body with the same layout as the transaction emails (card, inline styles, mailer.Palette colors).
func (v *View) HTML() (string, error) {
	var buf bytes.Buffer
	err := htmlTmpl.Execute(&buf, struct {
		*View
		P             any
		DefaultFooter string
	}{v, mailer.Palette, defaultFooter()})
	return buf.String(), err
}

var htmlTmpl = template.Must(template.New("report").Parse(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:24px 12px;background:{{.P.Page}};font-family:-apple-system,BlinkMacSystemFont,'PingFang SC','Helvetica Neue',Arial,sans-serif;color:{{.P.Text}};">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:600px;margin:0 auto;background:{{.P.Card}};border:1px solid {{.P.Border}};border-radius:14px;overflow:hidden;">
  <tr><td style="height:4px;background:{{.P.Neutral}};"></td></tr>
  <tr><td style="padding:22px 24px 8px;">
    <div style="font-size:13px;color:{{.P.Muted}};">{{.Kicker}}</div>
    <div style="margin-top:4px;font-size:20px;font-weight:600;">{{.Title}}</div>
    <div style="margin-top:16px;font-size:13px;color:{{.P.Muted}};">{{.Hero.Label}}</div>
    <div style="margin-top:2px;font-size:34px;font-weight:700;letter-spacing:-0.5px;">{{.Hero.Value}}{{if .Hero.Delta}} <span style="font-size:15px;font-weight:600;color:{{if .Hero.Up}}{{.P.Income}}{{else if .Hero.Down}}{{.P.Refund}}{{else}}{{.P.Muted}}{{end}};">{{.Hero.Delta}}</span>{{end}}</div>
    {{if .HeroNote}}<div style="margin-top:4px;font-size:12px;color:{{.P.Muted}};">{{.HeroNote}}</div>{{end}}
  </td></tr>
  {{if .Stats}}<tr><td style="padding:12px 24px 4px;">
    <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background:{{.P.Page}};border-radius:10px;">
      <tr>{{range .Stats}}<td style="padding:12px 14px;vertical-align:top;">
        <div style="font-size:12px;color:{{$.P.Muted}};">{{.Label}}</div>
        <div style="margin-top:2px;font-size:18px;font-weight:600;">{{.Value}}</div>
        {{if .Delta}}<div style="font-size:12px;color:{{if .Up}}{{$.P.Income}}{{else if .Down}}{{$.P.Refund}}{{else}}{{$.P.Muted}}{{end}};">{{.Delta}}</div>{{end}}
      </td>{{end}}</tr>
    </table>
  </td></tr>{{end}}
  {{range .Tables}}<tr><td style="padding:18px 24px 0;">
    <div style="font-size:12px;font-weight:600;color:{{$.P.Muted}};margin-bottom:6px;">{{.Title}}</div>
    <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="font-size:13px;border-collapse:collapse;">
      <tr>{{range $i, $c := .Columns}}<td style="padding:6px 4px 6px 0;font-size:11px;color:{{$.P.Muted}};{{if $i}}text-align:right;{{end}}white-space:nowrap;">{{$c}}</td>{{end}}</tr>
      {{range .Rows}}<tr>{{range $i, $c := .}}<td style="padding:7px 4px 7px 0;border-top:1px solid {{$.P.Border}};vertical-align:top;{{if $i}}text-align:right;{{else}}word-break:break-all;{{end}}">{{$c.Value}}{{if $c.Delta}}<div style="font-size:11px;color:{{if $c.Up}}{{$.P.Income}}{{else if $c.Down}}{{$.P.Refund}}{{else}}{{$.P.Muted}}{{end}};">{{$c.Delta}}</div>{{end}}</td>{{end}}</tr>{{end}}
    </table>
    {{if .Note}}<div style="margin-top:6px;font-size:11px;color:{{$.P.Muted}};">{{.Note}}</div>{{end}}
  </td></tr>{{end}}
  <tr><td style="padding:18px 24px 20px;font-size:11px;color:{{.P.Muted}};">{{if .Footer}}{{.Footer}}{{else}}{{.DefaultFooter}}{{end}}</td></tr>
</table>
</body></html>`))

// defaultFooter is the footer of the daily and weekly reports.
func defaultFooter() string {
	return i18n.T("Sent daily by HiwiKInsight · production data only", "由 HiwiKInsight 每日自动发送 · 只统计正式环境（production）")
}
