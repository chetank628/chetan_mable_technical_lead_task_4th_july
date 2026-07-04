# Mable Tracker (`tracker.js`)

A standalone, dependency-free, framework-agnostic web tracking script. It
collects browser-side analytics events and ships them to the Mable ingest API
(`POST /api/events`) using resilient, non-blocking transport.

It works on any site — load it with a `<script src>` tag, no build step.

## Quick start

```html
<script src="/tracker.js"></script>
<script>
  window.mable.init({
    endpoint: "http://localhost:8080/api/events",
    consent: false, // start with consent off; call setConsent(true) on accept
    debug: true,    // optional: console.debug tracing
  });
</script>
```

### Data layer (load-order safe)

You can queue commands before the script loads, Google-Tag-Manager style:

```html
<script>
  window.mableDataLayer = window.mableDataLayer || [];
  window.mableDataLayer.push(["identify", "user-123"]);
  window.mableDataLayer.push(["track", "Lead", { source: "hero" }]);
</script>
<script src="/tracker.js"></script>
<script>
  window.mable.init({ endpoint: "http://localhost:8080/api/events" });
</script>
```

`init()` drains the queue, then future `push()` calls execute immediately.

## API

| Method | Description |
| --- | --- |
| `init({ endpoint, consent?, batchSize?, flushIntervalMs?, debug?, userId? })` | Initialize. Patches history for SPA page views, binds click + lifecycle listeners, replays any persisted retry queue. |
| `identify(userId)` | Associate subsequent events with a user id. |
| `setConsent(granted: boolean)` | Grant/revoke consent. On grant, buffered + persisted events are flushed. |
| `track(type, props)` | Track a whitelisted event. Non-whitelisted types are dropped. |
| `pageView()` | Manually emit a `PageView`. |
| `addToCart(props)` / `checkout(props)` / `paymentInfoAdded(props)` / `purchase(props)` / `lead(props)` / `click(props)` | Convenience helpers for the canonical commerce events. |

### Whitelisted event types

The backend ingests only these (others are dropped at the API filter stage):

`PageView, Click, AddToCart, Checkout, PaymentInfoAdded, Purchase, Lead`

### Special property keys

- `amount` → promoted to the top-level `amount` field (number).
- `currency` → promoted to the top-level `currency` field (string).
- Everything else in `props` is **coerced to a string** and placed in
  `properties` (the API requires `map[string]string`). Objects/arrays are
  JSON-stringified automatically.

```js
window.mable.purchase({
  amount: 129.99,
  currency: "USD",
  order_id: "ORD-1001",
  items: [{ id: 42, qty: 2 }], // becomes a JSON string in properties.items
});
```

## Auto-tracked events

- **PageView** — on initial load, and on SPA route changes. The script patches
  `history.pushState`/`replaceState` and listens to `popstate`, so client-side
  navigation in React Router / Remix emits page views correctly. Duplicate
  consecutive paths are de-duped.
- **Click** — delegated listener for elements with a `data-track` attribute:

  ```html
  <button data-track="AddToCart" data-track-id="42" data-track-name="Sneaker">
    Add to cart
  </button>
  ```

  All `data-track-*` attributes are captured into `properties`. If the
  `data-track` value is itself a whitelisted type (e.g. `AddToCart`), that type
  is used; otherwise it falls back to `Click`.

## Transport & resilience

- **Non-blocking**: every public method is wrapped in `try/catch` and never
  throws into the host UI.
- **Batching**: events are buffered and flushed when `batchSize` (default 10)
  is reached or after `flushIntervalMs` (default 5s).
- **Unload-safe**: flushes via `navigator.sendBeacon` on `visibilitychange`
  (hidden) and `pagehide`; falls back to `fetch(..., { keepalive: true })`.
- **Credentialed**: fetches use `credentials: "include"` to match the API's
  CORS + cookie auth.
- **Offline retry**: failed sends (network down / 5xx) are persisted to
  `localStorage` and replayed on the `online` event and on next `init()`. The
  persisted queue is byte-capped (oldest dropped) to avoid unbounded growth.
- **Non-retryable handling**: `400` (malformed) and `413` (too large) responses
  are dropped rather than retried.
- **Consent gating**: with consent off, events are buffered/held and never
  transmitted; the backend also drops any event missing `consent: true`.

## Browser vs. server data split

| Field | Source | Why |
| --- | --- | --- |
| `user_agent` | Browser (`navigator.userAgent`) | Available client-side. |
| `language`, `timezone`, `screen`, `viewport` | Browser | Only the client knows these. |
| `referrer`, `url` | Browser | Page context. |
| `session_id` | Browser (`sessionStorage`) | Per-tab session. |
| `user_id` | Browser (via `identify()`) | App-supplied identity. |
| `timestamp` | Browser (`Date.toISOString()`, RFC3339) | Event time. |
| **IP address** | **Server** (`c.ClientIP()`) | Never trust a client-sent IP. |
| **Server receipt time** | **Server** (`_received_at`) | Authoritative ingest time. |
| **Geo (country/region)** | **Server** (derived from IP) | Requires server-side IP lookup. |

The browser deliberately does **not** send an IP or geo — those are derived
server-side from the connection, which is the only trustworthy source.

## Wire shape (matches the Go API `IngestEvent`)

```json
{
  "event_type": "AddToCart",
  "user_id": "user-123",
  "session_id": "b1f3...",
  "timestamp": "2026-06-28T12:34:56.789Z",
  "url": "http://localhost:5173/product/42",
  "referrer": "http://localhost:5173/",
  "user_agent": "Mozilla/5.0 ...",
  "amount": 0,
  "currency": "",
  "properties": { "id": "42", "name": "Sneaker", "timezone": "Europe/London" },
  "consent": true
}
```

The API accepts either a single object or an array (the tracker always sends an
array batch) and responds `202 { "accepted": n, "dropped": m }`.
