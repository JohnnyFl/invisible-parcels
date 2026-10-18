import { createFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Header } from "../components/Header";
import { levelClass } from "../components/LevelBadge";
import { ParcelTable } from "../components/ParcelTable";
import { ScanDialog } from "../components/ScanDialog";
import { api, formatTime, formatTTN, type IncomingRow, type ProblemRow } from "../lib/api";
import { requireRole } from "../lib/guard";
import { useLocationName, useVirtualNow } from "../lib/queries";
import { useInvalidateStream } from "../lib/useInvalidateStream";

export const Route = createFileRoute("/operator")({
  beforeLoad: ({ context }) => requireRole(context.queryClient, "operator"),
  component: Operator,
});

function Operator() {
  useInvalidateStream(true);
  const { now, clock } = useVirtualNow();
  const name = useLocationName();
  const asIs = clock?.mode === "as_is";
  const problems = useQuery({ queryKey: ["problems"], queryFn: () => api<ProblemRow[]>("/api/operator/problems") });
  const incoming = useQuery({ queryKey: ["incoming"], queryFn: () => api<IncomingRow[]>("/api/operator/incoming") });
  const physical = useQuery({ queryKey: ["physical"], queryFn: () => api<string[]>("/api/operator/physical") });
  const [scanning, setScanning] = useState<string | null>(null);
  const levelOf = new Map(problems.data?.map((r) => [r.ttn, r.level]));
  const expected = new Set(incoming.data?.map((r) => r.ttn));

  return (
    <div className="min-h-screen">
      <Header />
      <main className="mx-auto grid max-w-7xl gap-8 px-4 py-6 lg:grid-cols-[1fr_22rem]">
        <section aria-labelledby="problems-h">
          <h1 id="problems-h" className="mb-3 text-xl font-semibold">
            Проблемні посилки {problems.data && !asIs && <span className="font-normal text-ink-2">({problems.data.length})</span>}
          </h1>
          {asIs ? (
            <p className="border-y border-line bg-surface px-4 py-6 text-ink-2">
              Так система працює зараз: окремого списку проблемних посилок немає. Посилку, про яку не надійшло скану, оператор не побачить.
            </p>
          ) : problems.data?.length === 0 ? (
            <p className="border-y border-line bg-surface px-4 py-6 text-ink-2">Проблемних посилок немає. Усе, що прямує у відділення, є в списку надходжень.</p>
          ) : (
            problems.data && <ParcelTable rows={problems.data} now={now} />
          )}
        </section>

        <aside className="space-y-8">
          <section aria-labelledby="shelf-h">
            <h2 id="shelf-h" className="text-lg font-semibold">Склад</h2>
            <p className="mb-3 text-sm text-ink-2">Що фізично лежить на полицях. У демо замінює ручний сканер.</p>
            {physical.isError ? (
              <p className="text-sm text-lvl-red">Емулятор недоступний. Перевірте, що його запущено.</p>
            ) : (
              <ul className="divide-y divide-line border-y border-line bg-surface">
                {physical.data?.length === 0 && <li className="px-3 py-3 text-sm text-ink-2">Полиці порожні.</li>}
                {physical.data?.map((ttn) => {
                  const lvl = levelOf.get(ttn);
                  return (
                    <li key={ttn} className="flex items-center gap-3 px-3 py-2">
                      <span className={`h-2.5 w-2.5 shrink-0 rounded-full ${lvl ? levelClass[lvl].rail : "bg-line"}`} title={lvl ? "Є в списку проблемних" : undefined} />
                      <span className="ttn">{formatTTN(ttn)}</span>
                      <button className="ml-auto rounded border border-line px-3 py-1 text-sm hover:bg-bg" onClick={() => setScanning(ttn)}>
                        Сканувати
                      </button>
                    </li>
                  );
                })}
              </ul>
            )}
          </section>

          <section aria-labelledby="incoming-h">
            <h2 id="incoming-h" className="text-lg font-semibold">Очікувані надходження</h2>
            <p className="mb-3 text-sm text-ink-2">
              {asIs ? "Лише посилки з відсканованим завантаженням на рейс." : "Усі посилки, що за планом маршруту прямують у відділення."}
            </p>
            <ul className="divide-y divide-line border-y border-line bg-surface text-sm">
              {incoming.data?.length === 0 && <li className="px-3 py-3 text-ink-2">Надходжень не очікується.</li>}
              {incoming.data?.map((r) => (
                <li key={r.ttn} className="px-3 py-2">
                  <div className="flex justify-between gap-2">
                    <span className="ttn font-medium">{formatTTN(r.ttn)}</span>
                    <span>{formatTime(r.eta)}</span>
                  </div>
                  <div className="text-ink-2">з {name(r.from)}</div>
                </li>
              ))}
            </ul>
            {!asIs && expected.size > 0 && <p className="mt-2 text-xs text-ink-2">Час — прогноз за планом маршруту.</p>}
          </section>
        </aside>
      </main>
      <ScanDialog ttn={scanning} mode={clock?.mode} onClose={() => setScanning(null)} />
    </div>
  );
}
