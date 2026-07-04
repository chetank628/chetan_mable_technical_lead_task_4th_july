import { FiCheckCircle, FiInfo, FiX, FiXCircle } from "react-icons/fi";
import { useUiStore } from "~/stores/uiStore";

const styles = {
  success: "border-emerald-200 bg-emerald-50 text-emerald-800",
  error: "border-rose-200 bg-rose-50 text-rose-800",
  info: "border-slate-200 bg-white text-slate-800",
} as const;

const icons = {
  success: FiCheckCircle,
  error: FiXCircle,
  info: FiInfo,
} as const;

export function Toaster() {
  const toasts = useUiStore((s) => s.toasts);
  const dismiss = useUiStore((s) => s.dismissToast);

  if (!toasts.length) return null;

  return (
    <div
      aria-live="polite"
      className="fixed right-4 top-20 z-50 flex w-80 flex-col gap-2"
    >
      {toasts.map((t) => {
        const Icon = icons[t.kind];
        return (
          <div
            key={t.id}
            className={`flex items-start gap-2 rounded-lg border px-4 py-3 text-sm shadow-sm ${styles[t.kind]}`}
          >
            <Icon className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
            <span className="flex-1">{t.message}</span>
            <button
              type="button"
              onClick={() => dismiss(t.id)}
              aria-label="Dismiss"
              className="text-current/60 hover:text-current"
            >
              <FiX className="h-4 w-4" aria-hidden />
            </button>
          </div>
        );
      })}
    </div>
  );
}
