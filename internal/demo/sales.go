package demo

import (
	"math"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/store"
)

// App Store Connect reports for the demo apps. Days covered by SDK events are built from
// the generated installs and notifications; earlier days resample those days, scaled down
// to a smaller, younger app, so a year of history looks like the same business.

// pacific is the time zone of App Store Connect sales reports.
var pacific = func() *time.Location {
	if l, err := time.LoadLocation("America/Los_Angeles"); err == nil {
		return l
	}
	return time.FixedZone("PST", -8*3600)
}()

const (
	dailyReports  = 120 // days of DAILY sales reports kept, ending at the newest report
	reportDelay   = 2   // the newest daily report is for the day before yesterday (Pacific)
	commission    = 0.15
	funnelDays    = 90 // days of store funnel (analytics) reports
	funnelDelay   = 3
	monthlyLagDay = 5 // a MONTHLY report appears a few days after the month ends
)

type saleKey struct {
	bundle, sku, parentSKU, title, productType, category string
	country, currency, subscription, subPeriod, appleID  string
	customerPrice, ppu, usdPerUnit                       float64 // ppu: proceeds per unit, local currency
	refund                                               bool
}

type dayBook struct {
	keys  []saleKey // insertion order keeps resampling deterministic
	units map[saleKey]float64
}

type salesBook struct{ days map[string]*dayBook }

func newSalesBook() *salesBook { return &salesBook{days: map[string]*dayBook{}} }

func pacificDay(t time.Time) string { return t.In(pacific).Format("2006-01-02") }

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func (b *salesBook) add(day string, k saleKey, units float64) {
	d := b.days[day]
	if d == nil {
		d = &dayBook{units: map[saleKey]float64{}}
		b.days[day] = d
	}
	if _, ok := d.units[k]; !ok {
		d.keys = append(d.keys, k)
	}
	d.units[k] += units
}

func (b *salesBook) download(a *appSpec, rg region, at time.Time, units float64) {
	b.add(pacificDay(at), saleKey{bundle: a.bundle, sku: a.bundle, title: a.name, productType: "1F", category: "download",
		country: rg.code, currency: rg.currency, appleID: a.appleID}, units)
}

func (b *salesBook) sale(a *appSpec, p product, rg region, at time.Time, sign int, renewal bool) {
	local := localPrice(p.usd, rg)
	k := saleKey{bundle: a.bundle, sku: p.id, parentSKU: a.bundle, title: p.name, productType: "IA1", category: "iap",
		country: rg.code, currency: rg.currency, customerPrice: local, ppu: round2(local * (1 - commission)),
		usdPerUnit: local / rg.rate * (1 - commission), refund: sign < 0}
	if p.kind != "lifetime" {
		k.productType, k.category, k.subscription, k.subPeriod = "IAY", "subscription", "New", "1 Year"
		if renewal {
			k.subscription = "Renewal"
		}
		if p.kind == "monthly" {
			k.subPeriod = "1 Month"
		}
	}
	b.add(pacificDay(at), k, float64(sign))
}

// records turns booked units into report rows.
func (d *dayBook) records() []store.SalesRecord {
	if d == nil {
		return nil
	}
	var out []store.SalesRecord
	for _, k := range d.keys {
		u := d.units[k]
		if u == 0 {
			continue
		}
		proceeds := round2(u * k.usdPerUnit)
		out = append(out, store.SalesRecord{BundleID: k.bundle, SKU: k.sku, ParentSKU: k.parentSKU, Title: k.title,
			ProductType: k.productType, Category: k.category, Units: u, ProceedsPerUnit: k.ppu, ProceedsCurrency: k.currency,
			Proceeds: &proceeds, CustomerPrice: k.customerPrice, CustomerCurrency: k.currency, Country: k.country,
			AppleID: k.appleID, Subscription: k.subscription, SubPeriod: k.subPeriod})
	}
	return out
}

// probRound rounds randomly so the expected value is unchanged.
func (g *gen) probRound(v float64) float64 { return math.Floor(v + g.r.Float64()) }

func (g *gen) writeASC() error {
	var ascApps []store.ASCApp
	for _, a := range apps {
		ascApps = append(ascApps, store.ASCApp{AppleID: a.appleID, BundleID: a.bundle, SKU: a.bundle, Name: a.name})
	}
	if err := g.st.UpsertASCApps(ascApps); err != nil {
		return err
	}
	pt := g.now.In(pacific)
	today := time.Date(pt.Year(), pt.Month(), pt.Day(), 0, 0, 0, 0, time.UTC)
	latest := today.AddDate(0, 0, -reportDelay)
	histFrom := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -12, 0)
	windowFrom, _ := time.Parse("2006-01-02", pacificDay(g.start))
	g.resampleHistory(histFrom, windowFrom, today)

	for d := latest.AddDate(0, 0, -(dailyReports - 1)); !d.After(latest); d = d.AddDate(0, 0, 1) {
		day := d.Format("2006-01-02")
		rows := g.book.days[day].records()
		status := "ok"
		if len(rows) == 0 {
			status = "empty"
		}
		if err := g.st.ReplaceSales("DAILY", day, status, rows); err != nil {
			return err
		}
		g.sum.SalesRows += len(rows)
	}
	for m := histFrom; m.AddDate(0, 1, monthlyLagDay-1).Before(today); m = m.AddDate(0, 1, 0) {
		month := &dayBook{units: map[saleKey]float64{}}
		for d := m; d.Month() == m.Month(); d = d.AddDate(0, 0, 1) {
			if db := g.book.days[d.Format("2006-01-02")]; db != nil {
				for _, k := range db.keys {
					if _, ok := month.units[k]; !ok {
						month.keys = append(month.keys, k)
					}
					month.units[k] += db.units[k]
				}
			}
		}
		rows := month.records()
		if err := g.st.ReplaceSales("MONTHLY", m.Format("2006-01"), "ok", rows); err != nil {
			return err
		}
		g.sum.SalesRows += len(rows)
	}
	return g.writeStoreFunnel(today)
}

// resampleHistory fills the days before the SDK window with resampled window days,
// scaled by an app's growth since launch.
func (g *gen) resampleHistory(from, windowFrom, today time.Time) {
	var templates []string
	for i := 0; i < 30; i++ {
		templates = append(templates, windowFrom.AddDate(0, 0, i).Format("2006-01-02"))
	}
	for d := from; d.Before(windowFrom); d = d.AddDate(0, 0, 1) {
		ago := int(today.Sub(d).Hours() / 24)
		for _, a := range apps {
			if ago > a.launchedDays {
				continue
			}
			// grows from 35% of today's volume a year ago (or at launch) to 90% at the window start
			span := float64(min(a.launchedDays, 400) - Days)
			growth := 0.9 - 0.55*float64(ago-Days)/span
			if ago > a.launchedDays-14 {
				growth *= 1.8 // launch week
			}
			src := g.book.days[templates[g.r.IntN(len(templates))]]
			if src == nil {
				continue
			}
			for _, k := range src.keys {
				if k.bundle != a.bundle {
					continue
				}
				if u := g.probRound(src.units[k] * growth); u != 0 {
					g.book.add(d.Format("2006-01-02"), k, u)
				}
			}
		}
	}
}

// writeStoreFunnel writes the App Store analytics reports (impressions, product page views,
// first downloads by source and territory) that end in the booked downloads.
func (g *gen) writeStoreFunnel(today time.Time) error {
	type src struct {
		key                string
		share, view, first float64 // share of downloads, page views per impression, downloads per page view
	}
	srcs := []src{
		{"search", 0.55, 0.14, 0.42}, {"browse", 0.18, 0.06, 0.3}, {"app_referrer", 0.1, 0.32, 0.38},
		{"web_referrer", 0.11, 0.38, 0.28}, {"other", 0.06, 0.12, 0.3},
	}
	last := today.AddDate(0, 0, -funnelDelay)
	from := last.AddDate(0, 0, -(funnelDays - 1))
	downloads := func(bundle, day string) map[string]float64 {
		out := map[string]float64{}
		if db := g.book.days[day]; db != nil {
			for _, k := range db.keys {
				if k.bundle == bundle && k.category == "download" {
					out[k.country] += db.units[k]
				}
			}
		}
		return out
	}
	for _, a := range apps {
		avg := map[string]float64{} // average daily downloads per country
		for d := from; !d.After(last); d = d.AddDate(0, 0, 1) {
			for c, n := range downloads(a.bundle, d.Format("2006-01-02")) {
				avg[c] += n / funnelDays
			}
		}
		var recs []store.FunnelRecord
		for d := from; !d.After(last); d = d.AddDate(0, 0, 1) {
			day := d.Format("2006-01-02")
			byCountry := downloads(a.bundle, day)
			for _, rg := range regions {
				dl := byCountry[rg.code]
				for _, s := range srcs {
					first := int(g.probRound(dl * s.share))
					// page views follow interest, not only the downloads that happened that day
					pv := int(g.probRound((0.6*dl + 0.4*avg[rg.code]) * s.share / s.first))
					imp := int(g.probRound(float64(pv) / s.view * (0.8 + 0.4*g.r.Float64())))
					if imp == 0 && first == 0 {
						continue
					}
					recs = append(recs, store.FunnelRecord{Date: day, BundleID: a.bundle, Source: s.key, Territory: rg.code,
						Impressions: imp + int(g.probRound(float64(imp)*0.45)), ImpressionsUnique: imp,
						PageViews: pv + int(g.probRound(float64(pv)*0.3)), PageViewsUnique: pv,
						FirstDownloads: first, Redownloads: int(g.probRound(float64(first) * 0.08))})
				}
			}
		}
		processed := today.Format("2006-01-02")
		if err := g.st.ApplyAnalyticsInstance("demo-"+a.key+"-engagement", "App Store Discovery and Engagement", a.bundle, processed, true, recs); err != nil {
			return err
		}
		if err := g.st.ApplyAnalyticsInstance("demo-"+a.key+"-downloads", "App Downloads", a.bundle, processed, false, recs); err != nil {
			return err
		}
	}
	return nil
}
