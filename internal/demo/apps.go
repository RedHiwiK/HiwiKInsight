package demo

import (
	"fmt"
	"math/rand/v2"
)

// The three fictional demo apps. Everything about their behavior (screens, features,
// prices, conversion, errors, releases) lives here; generate.go only simulates it.

type product struct {
	id, name  string
	usd       float64
	kind      string // lifetime, monthly or yearly
	trialDays int    // free trial before the first charge (subscriptions only)
	weight    int    // how often users pick it on the paywall
}

func (p product) period() int {
	if p.kind == "monthly" {
		return 30
	}
	return 365
}

type paywallContext struct {
	key, label string
	weight     int
}

type screenDef struct {
	module, name string
	weight       int
	dwell        int // average seconds on screen
}

// feature is an app-specific event sent while a screen is open.
type feature struct {
	screen string
	prob   float64
	name   string
	params func(r *rand.Rand) map[string]string
}

type errorDef struct{ id, category, message string }

type release struct {
	version, build string
	day            int // day of the 90-day window the version shipped (negative: before the window)
}

type appSpec struct {
	key, bundle, name, color, appleID string
	installs                          int     // installs seen by the SDK over the window
	oldShare                          float64 // installs that predate the SDK: first seen early, downloaded months before
	launchedDays                      int     // days since the App Store launch (bounds the sales history)
	products                          []product
	contexts                          []paywallContext
	limit                             string  // what the free tier limits (limit.hit param)
	paywallRate                       float64 // chance a free user's session runs into a paywall
	onboardingPaywall                 float64 // chance the paywall follows onboarding
	legacySubs                        int     // subscribers from before the SDK, renewing without attribution
	onboarding                        []string
	screens                           []screenDef
	features                          []feature
	snapshot                          func(r *rand.Rand, paid bool, age int) map[string]string
	errors                            []errorDef
	spike                             string // error id spiking in the newest version
	releases                          []release
}

func oneOf(r *rand.Rand, v ...string) string { return v[r.IntN(len(v))] }

// bucket turns a growing count into the coarse buckets apps send in user.snapshot.
func bucket(n int) string {
	switch {
	case n <= 0:
		return "0"
	case n == 1:
		return "1"
	case n <= 5:
		return "2-5"
	case n <= 20:
		return "6-20"
	case n <= 100:
		return "21-100"
	}
	return "100+"
}

var apps = []*appSpec{
	{
		key: "pawprint", bundle: "com.example.pawprint", name: "Pawprint", color: "orange", appleID: "6500000101",
		installs: 400, oldShare: 0.3, launchedDays: 720,
		products: []product{{id: "com.example.pawprint.pro.lifetime", name: "Pawprint Pro (Lifetime)", usd: 9.99, kind: "lifetime", weight: 1}},
		contexts: []paywallContext{
			{"record_limit", "Record limit", 45}, {"settings_banner", "Settings banner", 25},
			{"export_limit", "Export limit", 15}, {"onboarding", "Onboarding", 0},
		},
		limit: "entries", paywallRate: 0.07, onboardingPaywall: 0.35,
		onboarding: []string{"welcome", "pet", "notifications"},
		screens: []screenDef{
			{"timeline", "timeline", 40, 40}, {"timeline", "entry_detail", 22, 25}, {"timeline", "search", 4, 20},
			{"entry", "new_entry", 26, 60}, {"pet", "pet_profile", 16, 30},
			{"health", "weight_chart", 7, 20}, {"health", "health_records", 6, 25},
			{"reminder", "reminder_list", 9, 15}, {"reminder", "new_reminder", 3, 35},
			{"settings", "settings", 6, 15}, {"settings", "export", 2, 30},
		},
		features: []feature{
			{"new_entry", 0.75, "entry.created", func(r *rand.Rand) map[string]string {
				return map[string]string{"module": "entry", "type": oneOf(r, "daily", "daily", "daily", "meal", "walk", "weight", "vet", "custom"), "photos": oneOf(r, "0", "1", "1", "2-5")}
			}},
			{"entry_detail", 0.12, "share.exported", func(r *rand.Rand) map[string]string {
				return map[string]string{"module": "timeline", "method": oneOf(r, "share_sheet", "share_sheet", "save_photo")}
			}},
			{"pet_profile", 0.08, "pet.added", func(r *rand.Rand) map[string]string {
				return map[string]string{"module": "pet", "species": oneOf(r, "dog", "dog", "cat", "cat", "rabbit", "other")}
			}},
			{"health_records", 0.4, "health.record_added", func(r *rand.Rand) map[string]string {
				return map[string]string{"module": "health", "kind": oneOf(r, "weight", "vaccine", "vet_visit", "medication")}
			}},
			{"weight_chart", 0.3, "health.record_added", func(r *rand.Rand) map[string]string {
				return map[string]string{"module": "health", "kind": "weight"}
			}},
			{"new_reminder", 0.8, "reminder.created", func(r *rand.Rand) map[string]string {
				return map[string]string{"module": "reminder", "kind": oneOf(r, "vaccine", "deworm", "grooming", "custom")}
			}},
			{"export", 0.6, "export.finished", func(r *rand.Rand) map[string]string {
				return map[string]string{"module": "settings", "format": oneOf(r, "pdf", "pdf", "csv"), "result": oneOf(r, "success", "success", "success", "success", "success", "success", "success", "failed")}
			}},
		},
		snapshot: func(r *rand.Rand, paid bool, age int) map[string]string {
			return map[string]string{"pets": oneOf(r, "1", "1", "1", "2-5"), "entries": bucket(age * 2 / 3), "is_pro": fmt.Sprint(paid),
				"reminders": oneOf(r, "on", "on", "off")}
		},
		errors: []errorDef{
			{"cloud_sync_failed", "sync", "CKError 3: network unavailable"},
			{"photo_load_failed", "media", "PHPhotosErrorDomain 3164: the asset is not available locally"},
			{"store_save_failed", "storage", "NSValidationErrorKey: name is required"},
		},
		spike:    "photo_load_failed",
		releases: []release{{"2.3.2", "41", -40}, {"2.4.0", "44", 38}, {"2.4.1", "46", 83}},
	},
	{
		key: "ledgerly", bundle: "com.example.ledgerly", name: "Ledgerly", color: "teal", appleID: "6500000202",
		installs: 250, oldShare: 0.35, launchedDays: 540,
		products: []product{
			{id: "com.example.ledgerly.plus.monthly", name: "Ledgerly Plus (Monthly)", usd: 2.99, kind: "monthly", weight: 2},
			{id: "com.example.ledgerly.plus.yearly", name: "Ledgerly Plus (Yearly)", usd: 19.99, kind: "yearly", trialDays: 7, weight: 3},
		},
		contexts: []paywallContext{
			{"record_limit", "Record limit", 35}, {"settings_banner", "Settings banner", 30},
			{"export_limit", "Export limit", 20}, {"onboarding", "Onboarding", 0},
		},
		limit: "accounts", paywallRate: 0.08, onboardingPaywall: 0.7, legacySubs: 24,
		onboarding: []string{"welcome", "currency", "first_account"},
		screens: []screenDef{
			{"dashboard", "dashboard", 40, 25}, {"dashboard", "report", 10, 40},
			{"ledger", "transactions", 28, 30}, {"ledger", "new_transaction", 30, 35}, {"ledger", "receipt_scan", 5, 25},
			{"assets", "accounts", 12, 20}, {"assets", "net_worth", 6, 25},
			{"budget", "budget_list", 9, 20}, {"budget", "budget_detail", 6, 30},
			{"settings", "settings", 5, 15}, {"settings", "export", 2, 25},
		},
		features: []feature{
			{"new_transaction", 0.85, "transaction.added", func(r *rand.Rand) map[string]string {
				return map[string]string{"module": "ledger", "kind": oneOf(r, "expense", "expense", "expense", "expense", "income", "transfer"), "method": oneOf(r, "manual", "manual", "quick_add", "widget")}
			}},
			{"receipt_scan", 0.85, "receipt.scanned", func(r *rand.Rand) map[string]string {
				return map[string]string{"module": "ledger", "result": oneOf(r, "success", "success", "success", "success", "failed")}
			}},
			{"accounts", 0.07, "account.added", func(r *rand.Rand) map[string]string {
				return map[string]string{"module": "assets", "type": oneOf(r, "bank", "bank", "card", "cash", "investment")}
			}},
			{"budget_list", 0.1, "budget.created", func(r *rand.Rand) map[string]string {
				return map[string]string{"module": "budget", "period": oneOf(r, "monthly", "monthly", "weekly")}
			}},
			{"report", 0.9, "report.viewed", func(r *rand.Rand) map[string]string {
				return map[string]string{"module": "dashboard", "range": oneOf(r, "month", "month", "month", "quarter", "year")}
			}},
			{"export", 0.6, "export.finished", func(r *rand.Rand) map[string]string {
				return map[string]string{"module": "settings", "format": "csv", "result": oneOf(r, "success", "success", "success", "success", "success", "failed")}
			}},
		},
		snapshot: func(r *rand.Rand, paid bool, age int) map[string]string {
			return map[string]string{"accounts": oneOf(r, "1", "2-5", "2-5", "6-20"), "transactions": bucket(age * 3), "budgets": oneOf(r, "0", "0", "1", "2-5"),
				"is_pro": fmt.Sprint(paid)}
		},
		errors: []errorDef{
			{"cloud_sync_failed", "sync", "CKError 9: request rate limited"},
			{"receipt_ocr_failed", "vision", "VNErrorDomain 9: no text found"},
			{"rate_fetch_failed", "network", "NSURLErrorDomain -1001: the request timed out"},
		},
		spike:    "rate_fetch_failed",
		releases: []release{{"1.8.0", "120", -25}, {"1.9.0", "126", 30}, {"1.9.1", "128", 61}, {"1.10.0", "131", 84}},
	},
	{
		key: "trailmark", bundle: "com.example.trailmark", name: "Trailmark", color: "purple", appleID: "6500000303",
		installs: 120, oldShare: 0.15, launchedDays: 150,
		products: []product{
			{id: "com.example.trailmark.pass.yearly", name: "Trailmark Pass (Yearly)", usd: 14.99, kind: "yearly", trialDays: 3, weight: 3},
			{id: "com.example.trailmark.pass.lifetime", name: "Trailmark Pass (Lifetime)", usd: 29.99, kind: "lifetime", weight: 2},
		},
		contexts: []paywallContext{
			{"record_limit", "Record limit", 55}, {"settings_banner", "Settings banner", 25}, {"onboarding", "Onboarding", 0},
		},
		limit: "stamps", paywallRate: 0.09, onboardingPaywall: 0.5, legacySubs: 5,
		onboarding: []string{"welcome", "location"},
		screens: []screenDef{
			{"map", "map", 40, 35}, {"map", "place_detail", 22, 25},
			{"stamp", "stamp_capture", 18, 30},
			{"collection", "stamp_book", 16, 30}, {"collection", "badges", 6, 15},
			{"trip", "trip_list", 8, 20}, {"trip", "new_trip", 3, 45},
			{"settings", "settings", 4, 15},
		},
		features: []feature{
			{"stamp_capture", 0.8, "stamp.collected", func(r *rand.Rand) map[string]string {
				return map[string]string{"module": "stamp", "method": oneOf(r, "gps", "gps", "gps", "photo", "manual"), "result": oneOf(r, "success", "success", "success", "success", "success", "failed")}
			}},
			{"new_trip", 0.8, "trip.created", func(r *rand.Rand) map[string]string {
				return map[string]string{"module": "trip", "kind": oneOf(r, "city", "city", "road_trip", "hike")}
			}},
			{"stamp_book", 0.1, "collection.shared", func(r *rand.Rand) map[string]string {
				return map[string]string{"module": "collection", "method": oneOf(r, "share_sheet", "save_photo")}
			}},
			{"badges", 0.25, "badge.earned", func(r *rand.Rand) map[string]string {
				return map[string]string{"module": "collection", "badge": oneOf(r, "first_stamp", "ten_stamps", "three_countries", "night_owl", "coast_to_coast")}
			}},
		},
		snapshot: func(r *rand.Rand, paid bool, age int) map[string]string {
			return map[string]string{"stamps": bucket(age / 2), "trips": oneOf(r, "0", "1", "1", "2-5"), "is_pro": fmt.Sprint(paid)}
		},
		errors: []errorDef{
			{"location_unavailable", "location", "kCLErrorDomain 0: location unknown"},
			{"map_tiles_failed", "network", "MKErrorDomain 3: loading throttled"},
		},
		spike:    "map_tiles_failed",
		releases: []release{{"1.2.0", "18", -10}, {"1.3.0", "22", 50}, {"1.3.1", "23", 85}},
	},
}

// region is a country with its App Store storefront, currency and a USD exchange rate.
type region struct {
	code, storefront, currency, language, locale string
	rate                                         float64 // currency units per USD
	weight                                       int
}

var regions = []region{
	{"US", "USA", "USD", "en", "en_US", 1, 38},
	{"GB", "GBR", "GBP", "en", "en_GB", 0.79, 10},
	{"CA", "CAN", "CAD", "en", "en_CA", 1.37, 8},
	{"AU", "AUS", "AUD", "en", "en_AU", 1.52, 6},
	{"DE", "DEU", "EUR", "de", "de_DE", 0.92, 9},
	{"FR", "FRA", "EUR", "fr", "fr_FR", 0.92, 5},
	{"NL", "NLD", "EUR", "nl", "nl_NL", 0.92, 3},
	{"JP", "JPN", "JPY", "ja", "ja_JP", 149, 8},
	{"KR", "KOR", "KRW", "ko", "ko_KR", 1370, 3},
	{"BR", "BRA", "BRL", "pt-BR", "pt_BR", 5.4, 4},
	{"MX", "MEX", "MXN", "es-MX", "es_MX", 18.5, 4},
	{"SE", "SWE", "SEK", "sv", "sv_SE", 10.6, 2},
}

// localPrice is the storefront price of a USD price, rounded like App Store price points.
func localPrice(usd float64, rg region) float64 {
	switch v := usd * rg.rate; {
	case rg.rate >= 100:
		return float64(int(v/100+0.5) * 100)
	case rg.rate >= 5:
		return float64(int(v+0.5)) - 0.01
	default:
		return float64(int(v)+1) - 0.01
	}
}

// weighted is a value picked with a relative weight.
type weighted struct {
	v      string
	weight int
}

var (
	devices = []weighted{
		{"iPhone18,1", 8}, {"iPhone18,2", 7}, {"iPhone18,3", 9}, {"iPhone18,4", 3},
		{"iPhone17,1", 10}, {"iPhone17,2", 8}, {"iPhone17,3", 10}, {"iPhone17,5", 4},
		{"iPhone16,1", 7}, {"iPhone16,2", 7}, {"iPhone15,4", 6}, {"iPhone15,2", 4}, {"iPhone15,3", 3},
		{"iPad16,3", 2}, {"iPad15,7", 2}, {"iPad14,1", 1},
	}
	osVersions = []weighted{{"27.0.1", 30}, {"27.0", 18}, {"26.6", 20}, {"26.5.1", 12}, {"26.4", 6}, {"18.7", 8}, {"18.6.2", 6}}
	// session sources: most sessions start from the icon
	sources = []string{"icon", "icon", "icon", "icon", "icon", "icon", "widget", "widget", "notification", "notification"}
	// failure reasons of purchase.failed, mostly user cancellations
	failReasons = []weighted{{"user_cancelled", 84}, {"product_missing", 6}, {"pending", 4}, {"system_error", 4}, {"verification", 2}}
)
