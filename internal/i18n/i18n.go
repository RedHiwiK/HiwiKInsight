// Package i18n picks the language of server-generated text: transaction emails,
// daily/weekly reports and alerts. The dashboard translates itself in the browser.
//
// Strings are written inline in both languages, English first:
//
//	i18n.T("Refund", "退款")
//
// Adding a language means adding a parameter; keep it that simple until there is a
// real need for message catalogs.
package i18n

import (
	"fmt"
	"sync/atomic"
)

type Lang string

const (
	EN Lang = "en"
	ZH Lang = "zh"
)

var current atomic.Value

func init() { current.Store(EN) }

// Set selects the language ("en" or "zh"); anything else falls back to English.
func Set(l string) {
	if Lang(l) == ZH {
		current.Store(ZH)
		return
	}
	current.Store(EN)
}

// Current returns the selected language.
func Current() Lang { return current.Load().(Lang) }

// T returns en or zh depending on the selected language.
func T(en, zh string) string {
	if Current() == ZH {
		return zh
	}
	return en
}

// Pair is a label in both languages, English first, for lookup tables.
type Pair [2]string

// String returns the label in the selected language.
func (p Pair) String() string { return T(p[0], p[1]) }

// Lookup returns the label for key in the selected language, or key itself when unknown.
func Lookup(m map[string]Pair, key string) string {
	if p, ok := m[key]; ok {
		return p.String()
	}
	return key
}

var currencySymbols = map[string]string{
	"CNY": "¥", "USD": "$", "HKD": "HK$", "TWD": "NT$", "MOP": "MOP$", "EUR": "€", "GBP": "£",
	"JPY": "JP¥", "KRW": "₩", "SGD": "S$", "AUD": "A$", "CAD": "C$", "CHF": "CHF ", "INR": "₹",
}

// Money formats an amount such as "$18.00" or "18.00 SEK".
func Money(amount float64, currency string) string {
	if sym, ok := currencySymbols[currency]; ok {
		if amount < 0 {
			return fmt.Sprintf("-%s%.2f", sym, -amount)
		}
		return fmt.Sprintf("%s%.2f", sym, amount)
	}
	return fmt.Sprintf("%.2f %s", amount, currency)
}
