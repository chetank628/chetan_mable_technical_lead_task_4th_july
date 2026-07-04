import { useEffect, useState } from "react";
import { FiActivity, FiRefreshCw } from "react-icons/fi";
import { api, ApiError } from "~/lib/api";
import type { Stats } from "~/lib/types";
import { useRequireAuth } from "~/hooks/useRequireAuth";
import { EmptyState, ErrorState, Spinner } from "~/components/states";

export function meta() {
  return [{ title: "Analytics — Mable Shop" }];
}

export default function Dashboard() {
  const { ready } = useRequireAuth();
  const [stats, setStats] = useState<Stats | null>(null);
  const [status, setStatus] = useState<"loading" | "ready" | "error">(
    "loading"
  );
  const [error, setError] = useState("");

  async function load() {
    setStatus("loading");
    setError("");
    try {
      const s = await api.stats({ granularity: "minute" });
      setStats(s);
      setStatus("ready");
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Failed to load stats");
      setStatus("error");
    }
  }

  useEffect(() => {
    if (ready) void load();
  }, [ready]);

  if (!ready) return <Spinner label="Checking your session…" />;
  if (status === "loading") return <Spinner label="Loading analytics…" />;
  if (status === "error") return <ErrorState message={error} onRetry={load} />;
  if (!stats) return null;

  const typeCounts = stats.type_counts || {};
  const typeEntries = Object.entries(typeCounts).sort((a, b) => b[1] - a[1]);
  const maxCount = typeEntries.reduce((m, [, c]) => Math.max(m, c), 0) || 1;

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-bold text-slate-900">
            <FiActivity aria-hidden /> Analytics
          </h1>
          <p className="text-sm text-slate-500">
            Live ingestion stats from the Mable pipeline (last 24h).
          </p>
        </div>
        <button
          type="button"
          onClick={load}
          className="flex items-center gap-1 rounded-lg border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100"
        >
          <FiRefreshCw aria-hidden /> Refresh
        </button>
      </div>

      <div className="mb-8 grid gap-4 sm:grid-cols-3">
        <StatCard label="Total events" value={stats.total_events.toLocaleString()} />
        <StatCard
          label="Avg capture (ms)"
          value={stats.avg_capture_ms.toFixed(2)}
        />
        <StatCard
          label="Avg event params"
          value={stats.avg_event_params.toFixed(1)}
        />
      </div>

      <section className="rounded-xl border border-slate-200 bg-white p-6">
        <h2 className="mb-4 text-lg font-semibold text-slate-800">
          Events by type
        </h2>
        {typeEntries.length === 0 ? (
          <EmptyState
            title="No events yet"
            hint="Accept tracking and browse the shop — events will appear here within a few seconds."
          />
        ) : (
          <ul className="flex flex-col gap-3">
            {typeEntries.map(([type, count]) => (
              <li key={type} className="flex items-center gap-3">
                <span className="w-36 shrink-0 text-sm font-medium text-slate-700">
                  {type}
                </span>
                <div className="h-6 flex-1 overflow-hidden rounded bg-slate-100">
                  <div
                    className="flex h-full items-center justify-end rounded bg-indigo-500 px-2 text-xs font-medium text-white"
                    style={{ width: `${Math.max(8, (count / maxCount) * 100)}%` }}
                  >
                    {count}
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>

      {stats.stage_rollups && stats.stage_rollups.length > 0 && (
        <section className="mt-8 rounded-xl border border-slate-200 bg-white p-6">
          <h2 className="mb-4 text-lg font-semibold text-slate-800">
            Pipeline stages
          </h2>
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="text-slate-500">
                <tr>
                  <th className="py-2 pr-4 font-medium">Stage</th>
                  <th className="py-2 pr-4 font-medium">In</th>
                  <th className="py-2 pr-4 font-medium">Out</th>
                  <th className="py-2 pr-4 font-medium">Dropped</th>
                  <th className="py-2 pr-4 font-medium">Errors</th>
                </tr>
              </thead>
              <tbody>
                {stats.stage_rollups.map((s) => (
                  <tr key={s.stage_name} className="border-t border-slate-100">
                    <td className="py-2 pr-4 font-medium text-slate-700">
                      {s.stage_name}
                    </td>
                    <td className="py-2 pr-4">{s.items_in}</td>
                    <td className="py-2 pr-4">{s.items_out}</td>
                    <td className="py-2 pr-4">{s.dropped}</td>
                    <td className="py-2 pr-4">{s.errors}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </div>
  );
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl border border-slate-200 bg-white p-5">
      <p className="text-sm text-slate-500">{label}</p>
      <p className="mt-1 text-2xl font-bold text-slate-900">{value}</p>
    </div>
  );
}
