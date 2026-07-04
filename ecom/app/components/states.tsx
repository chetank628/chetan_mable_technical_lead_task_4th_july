import { FiAlertTriangle, FiInbox, FiLoader } from "react-icons/fi";

// Reusable loading / empty / error UI so every route can show explicit states.

export function Spinner({ label = "Loading…" }: { label?: string }) {
  return (
    <div
      role="status"
      aria-live="polite"
      className="flex flex-col items-center justify-center gap-3 py-20 text-slate-500"
    >
      <FiLoader className="h-8 w-8 animate-spin" aria-hidden />
      <span>{label}</span>
    </div>
  );
}

export function EmptyState({
  title,
  hint,
  action,
}: {
  title: string;
  hint?: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 py-20 text-center text-slate-500">
      <FiInbox className="h-10 w-10" aria-hidden />
      <h2 className="text-lg font-medium text-slate-700">{title}</h2>
      {hint && <p className="max-w-sm text-sm">{hint}</p>}
      {action}
    </div>
  );
}

export function ErrorState({
  message,
  onRetry,
}: {
  message: string;
  onRetry?: () => void;
}) {
  return (
    <div
      role="alert"
      className="flex flex-col items-center justify-center gap-3 py-20 text-center"
    >
      <FiAlertTriangle className="h-10 w-10 text-amber-500" aria-hidden />
      <p className="max-w-sm text-slate-700">{message}</p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700"
        >
          Try again
        </button>
      )}
    </div>
  );
}
