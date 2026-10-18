import { createFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Header } from "../components/Header";
import { LevelBadge } from "../components/LevelBadge";
import { ParcelTable } from "../components/ParcelTable";
import { api, type Level, type ProblemRow } from "../lib/api";
import { requireRole } from "../lib/guard";
import { useVirtualNow } from "../lib/queries";
import { useInvalidateStream } from "../lib/useInvalidateStream";

export const Route = createFileRoute("/senior")({
  beforeLoad: ({ context }) => requireRole(context.queryClient, "senior"),
  component: Senior,
});

const order: Level[] = ["red", "orange", "yellow", "grey"];

function Senior() {
  useInvalidateStream(true);
  const { now, clock } = useVirtualNow();
  const { data, isPending } = useQuery({ queryKey: ["problems"], queryFn: () => api<ProblemRow[]>("/api/senior/problems") });
  const [filter, setFilter] = useState<Level | null>(null);
  const risky = (r: ProblemRow) => r.problems.some((p) => p.kind === "risky_return");
  const rows = (data ?? []).filter((r) => !filter || r.level === filter).sort((a, b) => Number(risky(b)) - Number(risky(a)));
  const counts = new Map(order.map((l) => [l, data?.filter((r) => r.level === l).length ?? 0]));

  return (
    <div className="min-h-screen">
      <Header />
      <main className="mx-auto max-w-7xl px-4 py-6">
        <h1 className="text-xl font-semibold">Проблемні посилки всіх відділень</h1>
        <p className="mb-4 text-sm text-ink-2">Ризикові повернення зверху: посилку повертають, хоча її не ідентифікували.</p>
        {clock?.mode === "as_is" ? (
          <p className="border-y border-line bg-surface px-4 py-6 text-ink-2">Так система працює зараз: зведеного списку проблем немає.</p>
        ) : (
          <>
            <div className="mb-4 flex flex-wrap gap-2" role="group" aria-label="Фільтр за рівнем">
              <button onClick={() => setFilter(null)} aria-pressed={!filter} className={`rounded border px-3 py-1 text-sm ${!filter ? "border-ink" : "border-line"}`}>
                Усі ({data?.length ?? 0})
              </button>
              {order.map((l) => (
                <button key={l} onClick={() => setFilter(l)} aria-pressed={filter === l} className={`rounded border px-1 py-0.5 ${filter === l ? "border-ink" : "border-transparent"}`}>
                  <LevelBadge level={l} label={`${counts.get(l)}`} />
                </button>
              ))}
            </div>
            {isPending ? (
              <p className="border-y border-line bg-surface px-4 py-6 text-ink-2">Завантаження…</p>
            ) : rows.length === 0 ? (
              <p className="border-y border-line bg-surface px-4 py-6 text-ink-2">Проблемних посилок немає.</p>
            ) : (
              <ParcelTable rows={rows} now={now} showBranch />
            )}
          </>
        )}
      </main>
    </div>
  );
}
