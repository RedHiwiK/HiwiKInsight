// Package fx converts foreign-currency amounts to the deployment's base currency.
// Rates come from open.er-api.com (free, no key) and are cached per calendar day.
package fx

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/tz"
)

// UpstreamURL is the rate source; the base currency code is appended. Tests may override it.
var UpstreamURL = "https://open.er-api.com/v6/latest/"

type Rates struct {
	base   string
	client *http.Client

	mu         sync.Mutex
	rates      map[string]float64 // 1 base = N units of the currency
	date       string
	refreshing bool
}

// New returns a converter to the given base currency (ISO 4217, e.g. "USD").
func New(base string) *Rates {
	return &Rates{base: base, client: &http.Client{Timeout: 10 * time.Second}}
}

// Base returns the base currency code.
func (r *Rates) Base() string { return r.base }

// ToBase converts an amount in thousandths of a currency unit (App Store "milli" prices)
// to the base currency. When the upstream is unavailable the last cached rates are used;
// ok is false when no rate is known at all.
func (r *Rates) ToBase(milli int64, currency string) (amount float64, ok bool) {
	return r.Convert(float64(milli)/1000, currency)
}

// Convert converts an amount in the given currency to the base currency.
func (r *Rates) Convert(amount float64, currency string) (float64, bool) {
	if currency == r.base {
		return amount, true
	}
	perBase := r.current()[currency]
	if perBase <= 0 {
		return 0, false
	}
	return amount / perBase, true
}

// Snapshot returns cached rates (1 base = N units) for the given codes without blocking;
// when the cache is not from today it refreshes once in the background.
func (r *Rates) Snapshot(codes ...string) map[string]float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.date != tz.Today() && !r.refreshing {
		r.refreshing = true
		go func() {
			r.current()
			r.mu.Lock()
			r.refreshing = false
			r.mu.Unlock()
		}()
	}
	out := make(map[string]float64, len(codes))
	for _, c := range codes {
		if c == r.base {
			out[c] = 1
		} else if v := r.rates[c]; v > 0 {
			out[c] = v
		}
	}
	return out
}

func (r *Rates) current() map[string]float64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	today := tz.Today()
	if r.date == today && len(r.rates) > 0 {
		return r.rates
	}
	if fresh, err := r.fetch(); err == nil {
		r.rates, r.date = fresh, today
	}
	return r.rates
}

func (r *Rates) fetch() (map[string]float64, error) {
	resp, err := r.client.Get(UpstreamURL + r.base)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body struct {
		Result string             `json:"result"`
		Rates  map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	if body.Result != "success" || len(body.Rates) == 0 {
		return nil, errors.New("fx: upstream returned no rates")
	}
	return body.Rates, nil
}
