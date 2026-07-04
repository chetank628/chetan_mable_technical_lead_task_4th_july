import { useEffect } from "react";
import { Outlet } from "react-router";
import { Header } from "~/components/Header";
import { ConsentBanner } from "~/components/ConsentBanner";
import { Toaster } from "~/components/Toaster";
import { useAuthStore } from "~/stores/authStore";

// The shared shell: header, consent banner, toaster, and the routed content.
// It restores the session once on mount via GET /auth/me (cookie-based).
export default function Shell() {
  const restore = useAuthStore((s) => s.restore);
  const status = useAuthStore((s) => s.status);

  useEffect(() => {
    if (status === "idle") {
      void restore();
    }
  }, [status, restore]);

  return (
    <div className="flex min-h-screen flex-col">
      <Header />
      <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-8">
        <Outlet />
      </main>
      <footer className="border-t border-slate-200 py-6 text-center text-xs text-slate-400">
        Mable demo shop — synthetic data, events flow to the local ingest
        pipeline.
      </footer>
      <Toaster />
      <ConsentBanner />
    </div>
  );
}
