# SDK integration

[HiwiKInsightKit](https://github.com/RedHiwiK/hiwikinsight-ios) is the Swift SDK that sends anonymous events to your HiwiKInsight server. It has no dependencies, targets iOS 17+ (Swift 6, Xcode 16+; the package also builds on macOS 14+), and stores events on disk until they are delivered. The wire format is specified in the SDK's [PROTOCOL.md](https://github.com/RedHiwiK/hiwikinsight-ios/blob/main/PROTOCOL.md), which is the only contract between SDK and server.

App Store revenue (server notifications and App Store Connect reports) does not need the SDK; the SDK adds usage analytics and purchase attribution.

## 1. Register the app on the server

Add the app to `config.yaml` and restart:

```yaml
apps:
  - key: pawprint                     # the SDK appKey
    bundle_id: com.example.pawprint
    name: Pawprint
    catalog: catalogs/pawprint.yaml   # optional, see event-catalog.md
    products:
      com.example.pawprint.pro.lifetime: Pawprint Pro (Lifetime)
    paywall_contexts:
      onboarding: Onboarding
      record_limit: Record limit
```

Events for a key that is not configured are rejected with HTTP 400 (the SDK then drops the batch).

## 2. Install

Swift Package Manager:

```swift
dependencies: [
    .package(url: "https://github.com/RedHiwiK/hiwikinsight-ios", from: "0.3.0"),
]
```

or in Xcode: **File > Add Package Dependencies...** with `https://github.com/RedHiwiK/hiwikinsight-ios`.

## 3. Start

Call `start` once at launch, for example in your `App` initializer:

```swift
import HiwiKInsightKit

HiwiKInsight.start(.init(appKey: "pawprint", endpoint: URL(string: "https://insight.example.com")!)) {
    // Optional user snapshot, sent as user.snapshot once per day on the first session.
    ["pets": "1", "entries": "21-100", "is_pro": "false"]
}
```

Events go to `<endpoint>/v1/events`. Configuration options (`sessionTimeout`, `flushInterval`, `flushThreshold`, `maxQueuedEvents`, `launchSourceWindow`, `hasPriorUsage`, `debugLogging`, `defaultsSuiteName`) are documented in the SDK README:

```swift
var config = HiwiKInsightConfiguration(appKey: "pawprint", endpoint: URL(string: "https://insight.example.com")!)
config.hasPriorUsage = LocalStore.hasExistingData   // your own check; see "Truly new users"
config.debugLogging = true
HiwiKInsight.start(config)
```

### Collected automatically

| Event | Params | Used by |
|---|---|---|
| `app.installed` | `original_download_ts`, `at_status`, `prior_usage` | `installs.acquired_at`, truly new users |
| `app.acquired` | `original_download_ts`, `at_status` | Late correction of the acquisition date |
| `app.updated` | `from`, `to` | |
| `session.started` | `source` | `daily_active.sessions`, `v_sessions.source` |
| `session.ended` | `duration_s` | Session duration metrics, `daily_active.duration_s` |
| `app.opened` | `source` | Re-entry from a widget or notification during a session |

Each batch also carries context: app version and build, OS and version, device model, locale, language, region, App Store storefront, environment, appearance and Dynamic Type size.

## 4. Custom events

```swift
HiwiKInsight.signal("entry.created", ["type": "walk", "photos": "2-5"])
```

Rules (enforced by the server; anything else is dropped or truncated):

- Names match `^[a-z0-9_.]{1,64}$`. Use `area.action` style: `entry.created`, `export.finished`.
- Params are string to string; at most 20 per event (sorted by key, the rest dropped), keys up to 40 characters, values up to 200.
- Do not use the reserved param names `app`, `type`, `ts`, `session`, `install_id`.
- Send bucketed, non-identifying values (`"0"`, `"1"`, `"2-5"`, `"6-20"`, `"21-100"`, `"100+"`), never free text or personal data.
- Document every event in the app's [event catalog](event-catalog.md); `describe` lists events that arrive but are not documented.

### Feature usage

Events that carry a `module` param (and are not screen events) appear in the `features` table of `module_usage`, grouped by module, event and feature. The feature label is the first non-empty of the `feature`, `source` or `action` params, and a `result` param of `success` (vs. anything else) gives a success rate:

```swift
HiwiKInsight.signal("export.finished", ["module": "settings", "format": "pdf", "result": "success"])
```

## 5. Screens

```swift
SettingsView().trackScreen("settings", module: "settings")
```

`trackScreen` records `screen.viewed` on appear and `screen.left` with `duration_s` on disappear. Attach it to standalone screens (the root of a push, sheet or full-screen cover). For tab roots that stay mounted, call `HiwiKInsight.screenViewed("timeline", module: "timeline")` (and optionally `screenLeft`) where the tab changes.

The `module` groups screens in the `module_usage` metric and the dashboard's Modules page. List your modules in the event catalog so the dashboard can label them.

## 6. Launch source

```swift
HiwiKInsight.setLaunchSource("widget_today")   // from .onOpenURL, a notification handler, a shortcut, ...
```

Called shortly after a new session starts (within `launchSourceWindow`, 1.5 s by default), it becomes the `source` of `session.started` (default `icon`); otherwise an `app.opened` event is recorded. Query it with `SELECT source, COUNT(*) FROM v_sessions ... GROUP BY source`.

## 7. Errors

```swift
HiwiKInsight.error(id: "sync.failed", category: "thrown-exception", message: "timeout")
```

This sends an `error` event with `id`, `category` and `message`. Use a stable `id` per error kind; the `errors` metric, the Errors page and alerts group by it. Ids listed in `alerts.payment_errors` (default `purchase`, `restore`, `products.load`) alert on their first occurrence each day; other ids alert when first seen ever or when they affect `alerts.spike_users` users within an hour. Keep `message` free of personal data.

## 8. User snapshot

The closure passed to `start` returns a dictionary sent as `user.snapshot` once per local calendar day on the first session. Use it for state rather than actions: number of items (bucketed), subscription state, settings toggles.

Report the membership state as `is_pro` (`"true"` or `"false"`). The server reads it from the latest snapshot for `v_user_summary.is_pro`, the Pro badge and the "Pro only" filter on the Users page, so members who bought before you integrated the SDK or restored a purchase still show up as Pro. Without it, only purchases attributed through `appAccountToken` are known.

## 9. Paywall funnel and purchase attribution

The `funnel` and `revenue` metrics and the transaction emails rely on these events. `purchase.started` comes from the SDK; the others are your own `signal` calls with the same `context` value:

| Event | Params | Sent by |
|---|---|---|
| `paywall.shown` | `context` | your app, when the paywall appears |
| `paywall.cta_tap` | `context`, `product` | your app, when the buy button is tapped |
| `purchase.started` | `context`, `product`, `token` | `HiwiKInsight.beginPurchase` |
| `purchase.success` | `context`, `product` | your app, after a verified transaction |
| `purchase.failed` | `context`, `product`, `reason` | your app; `reason` one of `user_cancelled`, `pending`, `verification`, `product_missing`, `system_error` |

`purchase.failed` with a reason other than `user_cancelled` or `pending` triggers a purchase alert (see [reports-and-alerts.md](reports-and-alerts.md)).

```swift
let context = "record_limit"
HiwiKInsight.signal("paywall.shown", ["context": context])

// buy button
HiwiKInsight.signal("paywall.cta_tap", ["context": context, "product": product.id])
let token = HiwiKInsight.beginPurchase(product: product.id, context: context)
do {
    let result = try await product.purchase(options: [.appAccountToken(token)])
    switch result {
    case .success(.verified(let transaction)):
        await transaction.finish()
        HiwiKInsight.signal("purchase.success", ["context": context, "product": product.id])
    case .success(.unverified):
        HiwiKInsight.signal("purchase.failed", ["context": context, "product": product.id, "reason": "verification"])
    case .userCancelled:
        HiwiKInsight.signal("purchase.failed", ["context": context, "product": product.id, "reason": "user_cancelled"])
    case .pending:
        HiwiKInsight.signal("purchase.failed", ["context": context, "product": product.id, "reason": "pending"])
    @unknown default:
        break
    }
} catch {
    HiwiKInsight.signal("purchase.failed", ["context": context, "product": product.id, "reason": "system_error"])
}
```

Map `context` values to display names under `apps[].paywall_contexts` in the server config.

### Apps that already use appAccountToken as an account ID

A transaction carries a single `appAccountToken`. If your app already passes its own fixed UUID per account (for example the account ID your backend assigns after Sign in with Apple), pass that UUID to `beginPurchase` instead of using a new one:

```swift
HiwiKInsight.signal("paywall.cta_tap", ["context": context, "product": product.id])
HiwiKInsight.beginPurchase(product: product.id, context: context, appAccountToken: accountID) // HiwiKInsightKit 0.3.0+
let result = try await product.purchase(options: [.appAccountToken(accountID)])
```

It records the same `purchase.started` event with `token` = your UUID (lowercase). One token then has many attempts, and the server picks one per transaction (see step 4 below). Both styles can be mixed across apps on one server.

A fixed account token lets anyone with database access link the installs of one account across devices and reinstalls. HiwiKInsight uses it only for attribution: the dashboard, `describe` and `insight user` never group installs by token. Mention it in your privacy policy if you use this style.

### How attribution works end to end

1. `beginPurchase` records `purchase.started` with `token` = a new UUID, or the UUID you pass (lowercase), and flushes immediately.
2. The server stores the attempt in `purchase_attempts` (token, app, env, install, context, product, time, app version, device, OS). A retried batch is not stored twice (same token, install and time).
3. You pass the same UUID to StoreKit as `appAccountToken`. Apple includes it in the signed transaction.
4. Apple sends an App Store Server Notification V2 to `/v1/appstore/notifications` (set up in [app-store-setup.md](app-store-setup.md)). The server verifies the signature chain, stores the transaction in `appstore_events` with `app_account_token`, the original transaction ID, the transaction reason and the purchase times, and converts the price to the base currency. The view `v_attribution` then picks one attempt: the **latest attempt with the same token in the same app, started no later than 5 minutes after the moment the user bought**. For a user-initiated purchase (`transactionReason` = `PURCHASE`, including a resubscription) that moment is the transaction's purchase time; for renewals, refunds and expiry it is the purchase time of the latest stored user-initiated transaction with the same `originalTransactionId`, or else the original purchase time. When no attempt qualifies, the latest attempt before the transaction itself is used; otherwise the transaction is `unattributed`. With a new UUID per purchase every token has one attempt, so this is a plain match. For first purchases (`ONE_TIME_CHARGE`, `SUBSCRIBED`) the server waits up to 5 seconds for the `purchase.started` event to be written.
5. The transaction email shows the source: paywall context, product, install day ("day N since install"), how many paywall views preceded it, app version and device. Renewals and refunds carry the original token, so they are attributed to the purchase that started them.
6. The `revenue` metric splits real revenue by context (`unattributed` when no token matched), product and storefront, and shows days from install to purchase; `funnel` adds verified sales and trials per context.

## 10. Environments

Each batch carries `env`: `production` (App Store), `sandbox` (TestFlight) or `xcode` (development builds). The SDK takes it from `AppTransaction.environment` and holds sending in Release builds until it is known, so TestFlight data does not leak into production. The server adds two safety nets:

- When App Store Connect is configured, it keeps a list of released builds (refreshed every 15 minutes). Events marked `production` from a version/build that was never released on the App Store are stored as `sandbox`, and earlier mislabeled data is reclassified.
- At startup, production events of installs that have also sent `sandbox` or `xcode` data (a development device) are moved to `sandbox`.

Metrics and the dashboard default to `production`; pass `env=sandbox` or `env=xcode` to see test data. Values other than the three are stored as `unknown`.

## 11. Truly new users

A "truly new user" is an install whose first-seen day equals its App Store first-download day. The SDK sends the original download time from `AppTransaction` (`original_download_ts`). People who installed the app before you added the SDK therefore count as existing users, not acquisition. If `AppTransaction` is unavailable but your app knows the device used it before (for example, existing local data), set `config.hasPriorUsage = true`; the server then treats that install as not new.

## 12. Opt-out and debugging

- `HiwiKInsight.setEnabled(false)` stops collection and deletes unsent events (for an in-app opt-out).
- `HiwiKInsight.installID` returns the anonymous install id, useful with `insight user <install_id>`.
- `await HiwiKInsight.flush()` sends the queue immediately.

## Server responses

| Status | Meaning |
|---|---|
| `202 {"accepted": n}` | Batch accepted. Events with an invalid name or id are skipped; events over `ingest.daily_cap` are dropped silently; duplicate event ids are ignored |
| `400` | Invalid JSON, unsupported `schema`, unknown app, invalid `install_id`, or not 1 to 200 events |
| `413` | Body over 256 KB, or over 2 MB after decompression |
| `415` | `Content-Encoding` other than none, `identity` or `deflate` (raw DEFLATE) |
| `503` | Write queue full; the SDK retries with backoff |

Event timestamps are corrected for client clock skew using `sent_at` (ignored when the skew exceeds 7 days) and are never later than the receive time.
