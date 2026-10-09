package demo

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/store"
	"github.com/RedHiwiK/HiwiKInsight/internal/tz"
)

// Summary counts what Generate wrote.
type Summary struct {
	Installs, Events, Notifications, SalesRows int
}

type gen struct {
	r     *rand.Rand
	st    *store.Store
	loc   *time.Location
	now   time.Time
	start time.Time // first day of the window, midnight in the reporting zone
	seq   int
	book  *salesBook
	notes []store.Event
	sum   Summary
}

// Generate fills an empty store with Days days of SDK events ending at now (in the
// reporting zone, see tz.Set), App Store notifications for the purchases they contain,
// and App Store Connect sales and store-funnel reports consistent with both.
func Generate(st *store.Store, now time.Time) (Summary, error) {
	loc := tz.Location()
	now = now.In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	g := &gen{r: rand.New(rand.NewPCG(Seed, 1)), st: st, loc: loc, now: now,
		start: today.AddDate(0, 0, -(Days - 1)), book: newSalesBook()}
	for _, a := range apps {
		if err := g.app(a); err != nil {
			return g.sum, fmt.Errorf("%s: %w", a.key, err)
		}
	}
	for _, n := range g.notes {
		if _, err := st.Record(n); err != nil {
			return g.sum, err
		}
	}
	g.sum.Notifications = len(g.notes)
	if err := g.writeASC(); err != nil {
		return g.sum, err
	}
	return g.sum, nil
}

// ---------- helpers ----------

func (g *gen) uuid() string {
	r := g.r
	return fmt.Sprintf("%08x-%04x-4%03x-%04x-%012x", r.Uint32(), r.Uint32()&0xffff, r.Uint32()&0xfff,
		0x8000|r.Uint32()&0x3fff, r.Uint64()&0xffffffffffff)
}

// eventID is unique and cheap; event IDs are never shown.
func (g *gen) eventID() string {
	g.seq++
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", g.seq)
}

func (g *gen) day(d int) time.Time { return g.start.AddDate(0, 0, d) }

func (g *gen) secs(lo, hi int) time.Duration {
	return time.Duration(lo+g.r.IntN(hi-lo+1)) * time.Second
}

func pick(r *rand.Rand, weights []int) int {
	sum := 0
	for _, w := range weights {
		sum += w
	}
	x := r.IntN(sum)
	for i, w := range weights {
		if x < w {
			return i
		}
		x -= w
	}
	return len(weights) - 1
}

func weights[T any](items []T, w func(T) int) []int {
	out := make([]int, len(items))
	for i, it := range items {
		out[i] = w(it)
	}
	return out
}

func pickV(r *rand.Rand, items []weighted) string {
	return items[pick(r, weights(items, func(x weighted) int { return x.weight }))].v
}

// ---------- users ----------

type user struct {
	a        *appSpec
	install  string
	env      string
	old      bool // installed before the SDK
	rg       region
	ctx      map[string]string
	firstDay int
	lag      int // days until the user updates to a new release
	download time.Time
	tier     int
	paid     bool
	seen     bool // app.installed sent
	stamps   int  // active days so far, drives user.snapshot sizes
	evs      []store.AnalyticsEvent
	ts       time.Time
	session  string
}

var tierProb = []float64{0.8, 0.45, 0.2, 0.07, 0} // heavy, medium, light, rare, one-time

func (g *gen) app(a *appSpec) error {
	var batches []*store.Batch
	extra := max(3, a.installs/25) // developer and TestFlight installs
	for i := 0; i < a.installs+extra; i++ {
		env := "production"
		if i >= a.installs {
			env = "sandbox"
			if i%3 == 0 {
				env = "xcode"
			}
		}
		batches = append(batches, g.user(a, env)...)
	}
	for _, b := range batches {
		g.sum.Events += len(b.Events)
	}
	g.sum.Installs += a.installs + extra
	if _, err := g.st.WriteBatches(batches); err != nil {
		return err
	}
	g.legacySubscribers(a)
	return nil
}

// growthDay picks the first day of a new install, with installs growing over the window.
func (g *gen) growthDay() int {
	for {
		d := g.r.IntN(Days)
		w := 0.55 + 0.9*float64(d)/Days
		if wd := g.day(d).Weekday(); wd == time.Saturday || wd == time.Sunday {
			w *= 1.25
		}
		if g.r.Float64()*1.8 < w {
			return d
		}
	}
}

func (g *gen) user(a *appSpec, env string) []*store.Batch {
	r := g.r
	u := &user{a: a, install: g.uuid(), env: env, lag: int(r.ExpFloat64() * 2.5)}
	u.old = env == "production" && r.Float64() < a.oldShare
	u.rg = regions[pick(r, weights(regions, func(x region) int { return x.weight }))]
	if u.old {
		u.firstDay = r.IntN(10)
	} else {
		u.firstDay = g.growthDay()
	}
	first := g.day(u.firstDay)
	if u.old {
		u.download = first.AddDate(0, 0, -30-r.IntN(min(a.launchedDays, 540)))
	} else {
		u.download = first.Add(time.Duration(7+r.IntN(14)) * time.Hour)
	}
	tw := []int{10, 18, 27, 25, 20}
	if u.old {
		tw = []int{22, 28, 25, 17, 8}
	}
	if env != "production" {
		tw = []int{30, 40, 30, 0, 0}
	}
	u.tier = pick(r, tw)
	churn := 3 + r.IntN(45)
	if u.old {
		churn = 30 + r.IntN(150)
	}
	u.ctx = map[string]string{
		"os": "iOS", "os_version": pickV(r, osVersions),
		"device": pickV(r, devices),
		"locale": u.rg.locale, "language": u.rg.language, "region": u.rg.code, "storefront": u.rg.storefront,
		"appearance": oneOf(r, "light", "light", "dark"), "text_size": oneOf(r, "M", "L", "L", "L", "XL", "XXL"),
	}

	var out []*store.Batch
	for d := u.firstDay; d < Days; d++ {
		if d > u.firstDay {
			p := tierProb[u.tier]
			if u.paid {
				p = max(p, 0.35)
			}
			if d-u.firstDay > churn {
				p *= 0.12
			}
			if r.Float64() >= p {
				continue
			}
		}
		b := g.simulateDay(u, d)
		if b == nil && d == u.firstDay {
			break // installed too late today to have opened the app yet
		}
		if b != nil {
			out = append(out, b)
		}
	}
	if env == "production" && !u.old && u.seen {
		g.book.download(a, u.rg, u.download, 1)
		if r.IntN(5) == 0 { // downloads that never opened the app
			g.book.download(a, u.rg, u.download, 1)
		}
	}
	return out
}

// version returns the app version a user runs on day d.
func (u *user) version(d int) release {
	v := u.a.releases[0]
	for _, rel := range u.a.releases {
		avail := rel.day
		if rel.day > u.firstDay || u.old {
			avail += u.lag
		}
		if avail <= d {
			v = rel
		}
	}
	return v
}

func (g *gen) add(u *user, name string, params map[string]string) {
	if u.ts.After(g.now) {
		return
	}
	u.evs = append(u.evs, store.AnalyticsEvent{ID: g.eventID(), Name: name, TS: u.ts.UnixMilli(), Session: u.session, Params: params})
	u.ts = u.ts.Add(g.secs(2, 20))
}

func (g *gen) simulateDay(u *user, d int) *store.Batch {
	r := g.r
	day := g.day(d)
	earliest, latest := day.Add(7*time.Hour), day.Add(22*time.Hour+30*time.Minute)
	if d == u.firstDay && !u.old && u.download.After(earliest) {
		earliest = u.download.Add(time.Duration(r.IntN(20)) * time.Minute)
	}
	if lim := g.now.Add(-40 * time.Minute); latest.After(lim) {
		latest = lim
	}
	if !latest.After(earliest) {
		return nil
	}
	u.ts = earliest.Add(time.Duration(r.Int64N(int64(latest.Sub(earliest))*6/10 + 1)))
	n := 1
	switch u.tier {
	case 0:
		n = 1 + r.IntN(3)
	case 1:
		n = 1 + r.IntN(2)
	default:
		if r.IntN(5) == 0 {
			n = 2
		}
	}
	u.evs = nil
	for s := 0; s < n && u.ts.Before(latest); s++ {
		g.session(u, d, s == 0)
		u.ts = u.ts.Add(time.Duration(40+r.IntN(240)) * time.Minute)
	}
	if len(u.evs) == 0 {
		return nil
	}
	u.stamps++
	rel := u.version(d)
	ctx := make(map[string]string, len(u.ctx)+2)
	for k, v := range u.ctx {
		ctx[k] = v
	}
	ctx["app_version"], ctx["build"] = rel.version, rel.build
	received := time.UnixMilli(u.evs[len(u.evs)-1].TS).Add(g.secs(1, 30))
	if received.After(g.now) {
		received = g.now
	}
	return &store.Batch{App: u.a.key, InstallID: u.install, Env: u.env, Context: ctx, ReceivedAt: received.UnixMilli(), Events: u.evs}
}

func (g *gen) session(u *user, d int, first bool) {
	r, a := g.r, u.a
	if !u.seen {
		u.session = ""
		g.add(u, "app.installed", map[string]string{"original_download_ts": strconv.FormatInt(u.download.UnixMilli(), 10)})
		u.seen = true
	}
	began := u.ts
	u.session = g.uuid()
	src := sources[r.IntN(len(sources))]
	if d == u.firstDay && first {
		src = "icon"
	}
	g.add(u, "session.started", map[string]string{"source": src})
	if first {
		snap := a.snapshot(r, u.paid, u.stamps+1)
		g.add(u, "user.snapshot", snap)
	}
	if d == u.firstDay && first && !u.old {
		g.onboarding(u)
	}
	screens := weights(a.screens, func(s screenDef) int { return s.weight })
	views := 1 + r.IntN(4)
	if u.tier == 0 {
		views += r.IntN(3)
	}
	for k := 0; k < views; k++ {
		g.screen(u, a.screens[pick(r, screens)])
	}
	if !u.paid && r.Float64() < a.paywallRate {
		g.paywall(u, a.contexts[pick(r, weights(a.contexts, func(c paywallContext) int { return c.weight }))].key)
	}
	g.maybeError(u, d)
	g.add(u, "session.ended", map[string]string{"duration_s": strconv.Itoa(int(u.ts.Sub(began).Seconds()) + 3)})
}

func (g *gen) onboarding(u *user) {
	r := g.r
	steps := u.a.onboarding
	done := len(steps)
	if r.IntN(7) == 0 {
		done = r.IntN(len(steps)) // left during onboarding
	}
	for i, step := range steps[:done] {
		name := "welcome"
		if i > 0 {
			name = map[string]string{"pawprint": "pet_setup", "ledgerly": "currency_setup", "trailmark": "location_permission"}[u.a.key]
		}
		g.add(u, "screen.viewed", map[string]string{"screen": name, "module": "onboarding"})
		dwell := g.secs(4, 25)
		u.ts = u.ts.Add(dwell)
		g.add(u, "onboarding.step", map[string]string{"module": "onboarding", "step": step})
		g.add(u, "screen.left", map[string]string{"screen": name, "module": "onboarding", "duration_s": strconv.Itoa(int(dwell.Seconds()) + 2)})
	}
	if done == len(steps) {
		g.add(u, "onboarding.completed", nil)
		if r.Float64() < u.a.onboardingPaywall {
			g.paywall(u, "onboarding")
		}
	}
}

func (g *gen) screen(u *user, sc screenDef) {
	r := g.r
	g.add(u, "screen.viewed", map[string]string{"screen": sc.name, "module": sc.module})
	dwell := max(2, sc.dwell/2+r.IntN(sc.dwell+1))
	u.ts = u.ts.Add(time.Duration(dwell/2) * time.Second)
	for _, f := range u.a.features {
		if f.screen == sc.name && r.Float64() < f.prob {
			if f.name == "export.finished" && !u.paid {
				// exporting is a Pro feature: free users run into the paywall instead
				g.paywall(u, "export_limit")
				continue
			}
			g.add(u, f.name, f.params(r))
		}
	}
	u.ts = u.ts.Add(time.Duration(dwell-dwell/2) * time.Second)
	g.add(u, "screen.left", map[string]string{"screen": sc.name, "module": sc.module, "duration_s": strconv.Itoa(dwell)})
}

func (g *gen) maybeError(u *user, d int) {
	r, a := g.r, u.a
	rel := u.version(d)
	newest := a.releases[len(a.releases)-1]
	if a.spike != "" && rel.version == newest.version && d >= Days-5 && r.IntN(5) == 0 {
		for _, e := range a.errors {
			if e.id == a.spike {
				g.add(u, "error", map[string]string{"id": e.id, "category": e.category, "message": e.message})
			}
		}
		return
	}
	if r.IntN(45) == 0 {
		e := a.errors[r.IntN(len(a.errors))]
		g.add(u, "error", map[string]string{"id": e.id, "category": e.category, "message": e.message})
	}
}

// ---------- paywall and purchases ----------

func (g *gen) paywall(u *user, ctx string) {
	if u.paid {
		return
	}
	r, a := g.r, u.a
	switch ctx {
	case "record_limit":
		g.add(u, "limit.hit", map[string]string{"limit": a.limit, "context": ctx})
	case "export_limit":
		g.add(u, "limit.hit", map[string]string{"limit": "export", "context": ctx})
	}
	g.add(u, "screen.viewed", map[string]string{"screen": "paywall", "module": "paywall"})
	g.add(u, "paywall.shown", map[string]string{"context": ctx})
	began := u.ts
	u.ts = u.ts.Add(g.secs(3, 30))
	p := a.products[pick(r, weights(a.products, func(p product) int { return p.weight }))]
	cta := 0.3
	if ctx == "onboarding" {
		cta = 0.18
	}
	if r.Float64() < cta {
		g.add(u, "paywall.cta_tap", map[string]string{"context": ctx, "product": p.id})
		if r.Float64() < 0.85 {
			token := g.uuid()
			g.add(u, "purchase.started", map[string]string{"context": ctx, "product": p.id, "token": token})
			u.ts = u.ts.Add(g.secs(5, 40))
			success := 0.55
			if p.trialDays > 0 {
				success = 0.72
			}
			if r.Float64() < success {
				g.add(u, "purchase.success", map[string]string{"context": ctx, "product": p.id})
				u.paid = true
				g.purchase(u.a, u.rg, u.env, p, token, u.ts)
			} else {
				reason := pickV(r, failReasons)
				g.add(u, "purchase.failed", map[string]string{"context": ctx, "product": p.id, "reason": reason})
				if reason == "product_missing" {
					g.add(u, "error", map[string]string{"id": "products.load", "category": "purchase", "message": "StoreKit returned no products for the requested IDs"})
				}
			}
		}
	}
	g.add(u, "screen.left", map[string]string{"screen": "paywall", "module": "paywall", "duration_s": strconv.Itoa(int(u.ts.Sub(began).Seconds()))})
}

// notify queues an App Store server notification and books the sale for the reports.
func (g *gen) notify(a *appSpec, rg region, env, typ, subtype string, p product, token, txn string, at time.Time, sign int, charged bool, renewal bool) {
	if at.After(g.now) || env == "xcode" { // StoreKit testing in Xcode sends no server notifications
		return
	}
	appleEnv := "Production"
	if env != "production" {
		appleEnv = "Sandbox"
	}
	var milli int64
	amount := 0.0
	if charged {
		local := localPrice(p.usd, rg)
		milli = int64(math.Round(local * 1000))
		amount = math.Round(local/rg.rate*100) / 100
	}
	g.notes = append(g.notes, store.Event{NotificationUUID: g.uuid(), BundleID: a.bundle, Environment: appleEnv,
		NotificationType: typ, Subtype: subtype, ProductID: p.id, TransactionID: txn, Storefront: rg.storefront,
		Currency: rg.currency, PriceMilli: milli, RevenueSign: sign, Amount: &amount, SignedAt: at.UnixMilli(), AppAccountToken: token})
	if appleEnv == "Production" && charged && sign != 0 {
		g.book.sale(a, p, rg, at, sign, renewal)
	}
}

func (g *gen) txnID() string { return strconv.FormatInt(2000000900000000+g.r.Int64N(99999999), 10) }

// purchase emits the notifications that follow a successful purchase, up to now.
func (g *gen) purchase(a *appSpec, rg region, env string, p product, token string, at time.Time) {
	r := g.r
	txn := g.txnID()
	at = at.Add(g.secs(1, 8))
	if p.kind == "lifetime" {
		g.notify(a, rg, env, "ONE_TIME_CHARGE", "", p, token, txn, at, 1, true, false)
		if r.IntN(18) == 0 {
			g.notify(a, rg, env, "REFUND", "", p, token, txn, at.Add(time.Duration(2+r.IntN(12))*24*time.Hour), -1, true, false)
		}
		return
	}
	t := at
	if p.trialDays > 0 {
		g.notify(a, rg, env, "SUBSCRIBED", "INITIAL_BUY", p, token, txn, t, 1, false, false)
		end := t.Add(time.Duration(p.trialDays) * 24 * time.Hour)
		if r.Float64() < 0.42 { // cancelled during the trial
			g.notify(a, rg, env, "DID_CHANGE_RENEWAL_STATUS", "AUTO_RENEW_DISABLED", p, token, txn,
				t.Add(time.Duration(r.Int64N(int64(end.Sub(t))))), 0, false, false)
			g.notify(a, rg, env, "EXPIRED", "VOLUNTARY", p, token, txn, end, 0, false, false)
			return
		}
		t = end
		g.notify(a, rg, env, "DID_RENEW", "", p, token, txn, t, 1, true, false)
	} else {
		g.notify(a, rg, env, "SUBSCRIBED", "INITIAL_BUY", p, token, txn, t, 1, true, false)
	}
	if p.kind == "yearly" && r.IntN(25) == 0 {
		g.notify(a, rg, env, "REFUND", "", p, token, txn, t.Add(time.Duration(1+r.IntN(10))*24*time.Hour), -1, true, false)
		return
	}
	g.renewals(a, rg, env, p, token, txn, t)
}

// renewals continues a subscription from its last charge at t until it churns or reaches now.
func (g *gen) renewals(a *appSpec, rg region, env string, p product, token, txn string, t time.Time) {
	r := g.r
	churn := 0.14
	if p.kind == "yearly" {
		churn = 0.3
	}
	for {
		next := t.AddDate(0, 0, p.period())
		if next.After(g.now) {
			return
		}
		if r.Float64() < churn {
			g.notify(a, rg, env, "DID_CHANGE_RENEWAL_STATUS", "AUTO_RENEW_DISABLED", p, token, txn,
				next.Add(-time.Duration(1+r.IntN(min(p.period()-1, 60)))*24*time.Hour), 0, false, false)
			g.notify(a, rg, env, "EXPIRED", "VOLUNTARY", p, token, txn, next, 0, false, false)
			return
		}
		g.notify(a, rg, env, "DID_RENEW", "", p, token, txn, next, 1, true, true)
		t = next
	}
}

// legacySubscribers renew without attribution: they subscribed before the app adopted the SDK.
func (g *gen) legacySubscribers(a *appSpec) {
	r := g.r
	var subs []product
	for _, p := range a.products {
		if p.kind != "lifetime" {
			subs = append(subs, p)
		}
	}
	if len(subs) == 0 {
		return
	}
	for i := 0; i < a.legacySubs; i++ {
		p := subs[r.IntN(len(subs))]
		if p.kind == "yearly" && r.IntN(2) == 0 {
			p = subs[0] // most long-time subscribers are monthly
		}
		rg := regions[pick(r, weights(regions, func(x region) int { return x.weight }))]
		last := g.start.Add(-time.Duration(r.Int64N(int64(p.period()) * int64(24*time.Hour))))
		last = last.Add(time.Duration(r.IntN(86400)) * time.Second)
		g.renewals(a, rg, "production", p, "", g.txnID(), last)
	}
}
