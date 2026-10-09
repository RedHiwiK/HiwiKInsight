package mailer

// Palette holds the colors of every email (transactions, reports, alerts).
// HTML email only supports inline styles, so colors are defined once here.
var Palette = struct {
	Page, Card, Border, Text, Muted, Income, Refund, Neutral, Badge, OnBadge string
}{
	Page: "#f4f4f5", Card: "#ffffff", Border: "#e4e4e7", Text: "#18181b", Muted: "#71717a",
	Income: "#16a34a", Refund: "#dc2626", Neutral: "#2563eb", Badge: "#f59e0b", OnBadge: "#ffffff",
}
