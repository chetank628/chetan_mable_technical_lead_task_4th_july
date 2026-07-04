import {
  isRouteErrorResponse,
  Links,
  Meta,
  Outlet,
  Scripts,
  ScrollRestoration,
} from "react-router";
import type { Route } from "./+types/root";
import "./app.css";

export const links: Route.LinksFunction = () => [
  { rel: "preconnect", href: "https://cdn.dummyjson.com" },
];

export function Layout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <meta charSet="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <title>Mable Shop</title>
        <Meta />
        <Links />
        {/*
          Load the standalone tracker exactly as any third-party site would.
          It is initialized from the ConsentBanner / app bootstrap, gated on
          user consent. The script never blocks rendering (it self-defers work).
        */}
        <script src="/tracker.js" defer></script>
      </head>
      <body suppressHydrationWarning>
        {children}
        <ScrollRestoration />
        <Scripts />
      </body>
    </html>
  );
}

export default function App() {
  return <Outlet />;
}

// Root-level error boundary: catches anything not handled by a route boundary
// and beacons an error event (best-effort) so failures are observable.
export function ErrorBoundary({ error }: Route.ErrorBoundaryProps) {
  let title = "Something went wrong";
  let detail = "An unexpected error occurred.";

  if (isRouteErrorResponse(error)) {
    title = `${error.status} ${error.statusText}`;
    detail = error.data || detail;
  } else if (error instanceof Error) {
    detail = error.message;
  }

  // Best-effort error beacon (never throws).
  if (typeof window !== "undefined" && window.mable) {
    try {
      window.mable.track("Click", {
        kind: "client_error",
        message: detail,
        url: window.location.href,
      });
    } catch {
      /* no-op */
    }
  }

  return (
    <main className="mx-auto flex min-h-screen max-w-lg flex-col items-center justify-center gap-4 px-6 text-center">
      <h1 className="text-2xl font-semibold text-slate-900">{title}</h1>
      <p className="text-slate-600">{detail}</p>
      <a
        href="/"
        className="rounded-lg bg-indigo-600 px-5 py-2.5 font-medium text-white hover:bg-indigo-700"
      >
        Back to shop
      </a>
    </main>
  );
}
