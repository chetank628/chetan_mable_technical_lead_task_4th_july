# Mable Shop (`ecom/`)

A demo e-commerce SPA built with **React Router 7 (Remix) in SPA mode**, React
19, Zustand, Tailwind v4 and Vite. It exercises the Mable tracking pipeline
end-to-end: auth via the Go API, a consent-gated tracker, and a live analytics
dashboard reading `/api/stats`.

## Stack

- **React Router 7** (framework mode, `ssr: false`) on **Vite** — serves on
  `:5173` to match the API's CORS allow-list.
- **React 19**, **TypeScript**, **Zustand** (state), **Tailwind CSS v4**,
  **React Icons**.
- **pnpm** for dependency management.

## Prerequisites

- Node 20+, pnpm 8+
- The Go API running on `:8080` (see `../api`).

## Run locally (all three pieces)

```bash
# 1. API (terminal A)
cd api && go run .

# 2. Frontend (terminal B)
cd ecom && pnpm install && pnpm dev      # http://localhost:5173
```

The standalone tracker is served from `ecom/public/tracker.js` (a copy of
`../scripts/tracker.js`) and loaded via a `<script src>` tag in `app/root.tsx`,
exactly as any third-party site would embed it.

### Environment

`.env` (already present):

```
VITE_API_BASE=http://localhost:8080
VITE_TRACKER_ENDPOINT=http://localhost:8080/api/events
```

## Verifying the full journey

1. Open `http://localhost:5173` and **Accept tracking** in the consent banner.
2. **Sign up** (`/signup`) — email with `@`, password ≥ 8 chars.
3. **Browse** the product grid (DummyJSON), search/filter, open a product.
4. **Add to cart**, adjust quantities in `/cart`.
5. **Checkout** (`/checkout`) — multi-step shipping → payment → review → place
   order → success → redirect home.
6. Open **Stats** (`/dashboard`) to see events land via `GET /api/stats`
   (cookie-gated), or check raw pipeline metrics at
   `http://localhost:8080/metrics`.

Events emitted along the way: `PageView` (auto, on every route change),
`Lead` (signup/login), `AddToCart`, `Checkout`, `PaymentInfoAdded`, `Purchase`.

## Architecture

```
app/
  root.tsx              # HTML shell, loads /tracker.js, root error boundary
  routes.ts             # route config (SPA)
  routes/
    _shell.tsx          # header + consent banner + toaster + session restore
    home.tsx            # product library (search/filter/add-to-cart)
    product.tsx         # product detail
    cart.tsx            # quantities, remove, totals
    checkout.tsx        # multi-step, auth-gated, tracks each step
    dashboard.tsx       # /api/stats analytics
    login.tsx / signup.tsx
  stores/
    authStore.ts        # user from /auth/me (never the JWT)
    cartStore.ts        # persisted to localStorage (survives auth/refresh)
    uiStore.ts          # consent (persisted) + toasts
  lib/
    api.ts              # credentialed API client + DummyJSON catalog
    tracker.ts          # typed wrapper around window.mable
    types.ts            # shared DTOs (mirror the Go API)
  components/           # Header, ConsentBanner, Toaster, states
  hooks/useRequireAuth  # protected-route redirect
```

## How the contracts are honoured

- **Auth**: every fetch uses `credentials: "include"`; the JWT lives only in the
  `HttpOnly` `mable_session` cookie and is never read by JS. Session is restored
  on load via `GET /auth/me`.
- **Properties are strings**: the tracker coerces every `properties` value to a
  string (objects → JSON), matching the API's `map[string]string`.
- **Consent**: nothing is transmitted with `consent:true` until the user
  accepts; the backend also drops events lacking consent.
- **Tracked types only**: `PageView, Click, AddToCart, Checkout,
  PaymentInfoAdded, Purchase, Lead`.

## Edge cases covered

- **API down / network failure** — tracker queues to `localStorage` and replays
  on `online`; UI shows error states with retry, never blocks.
- **Oversized/invalid payloads** — `413`/`400` responses are dropped (not
  retried) by the tracker; the API client surfaces a readable message.
- **Consent not granted** — no events sent; held in the tracker queue.
- **Session expiry / 401** — protected routes redirect to `/login?next=…`; the
  **cart is preserved** (separate localStorage store).
- **Refresh mid-checkout** — session re-restored via `/auth/me`, cart restored
  from localStorage.
- **Duplicate rapid clicks** — server-side dedup; add-to-cart is idempotent per
  product (increments quantity).
- **Empty cart** — checkout is blocked with an empty state; empty product API
  result shows an empty state.
- **SPA route changes** — the tracker patches `history.pushState`/`popstate`,
  so client-side navigation emits `PageView` correctly.

## Scripts

| Command | Description |
| --- | --- |
| `pnpm dev` | Start the dev server on `:5173`. |
| `pnpm build` | Production build (SPA → `build/client/`). |
| `pnpm start` | Preview the production build on `:5173`. |
| `pnpm typecheck` | Run typegen + `tsc`. |

> Scope: build + run locally. Cloud deployment is out of scope this round.
