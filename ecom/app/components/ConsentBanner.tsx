import { useEffect } from "react";
import { tracker } from "~/lib/tracker";
import { useUiStore } from "~/stores/uiStore";

// Consent gate. Tracking is initialized with the persisted consent decision;
// no events are transmitted with consent:true until the user accepts. The
// backend also drops any event lacking consent, so this is defence-in-depth.
export function ConsentBanner() {
  const consent = useUiStore((s) => s.consent);
  const setConsent = useUiStore((s) => s.setConsent);

  // Initialize the global tracker once, reflecting the stored decision.
  useEffect(() => {
    tracker.init(consent === "granted");
  }, [consent]);

  if (consent !== "unset") return null;

  return (
    <div
      role="dialog"
      aria-label="Cookie and tracking consent"
      className="fixed inset-x-0 bottom-0 z-40 border-t border-slate-200 bg-white px-4 py-4 shadow-[0_-4px_20px_rgba(0,0,0,0.06)]"
    >
      <div className="mx-auto flex max-w-4xl flex-col items-start gap-3 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-sm text-slate-600">
          We use a first-party analytics tracker to understand how this demo
          shop is used. Events are <strong>synthetic demo data</strong> sent to
          the local Mable pipeline. No tracking happens until you accept.
        </p>
        <div className="flex shrink-0 gap-2">
          <button
            type="button"
            onClick={() => setConsent(false)}
            className="rounded-lg border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100"
          >
            Decline
          </button>
          <button
            type="button"
            onClick={() => setConsent(true)}
            className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700"
          >
            Accept tracking
          </button>
        </div>
      </div>
    </div>
  );
}
