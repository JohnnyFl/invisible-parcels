import { createFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Header } from "../components/Header";
import { levelClass } from "../components/LevelBadge";
import { ParcelTable } from "../components/ParcelTable";
import { ScanDialog } from "../components/ScanDialog";
import { api, formatTime, formatTTN, type IncomingRow, type ProblemRow, type ShelfRow } from "../lib/api";
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
  const physical = useQuery({ queryKey: ["physical"], queryFn: () => api<ShelfRow[]>("/api/operator/physical") });
  const [scanning, setScanning] = useState<string | null>(null);
  const levelOf = new Map(problems.data?.map((r) => [r.ttn, r.level]));
  const unregistered = asIs ? 0 : (physical.data?.filter((r) => !r.registered).length ?? 0);

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
          ) : problems.isPending ? (
            <Loading />
          ) : problems.data?.length === 0 ? (
            <p className="border-y border-line bg-surface px-4 py-6 text-ink-2">Проблемних посилок немає. Усе, що прямує у відділення, є в списку надходжень.</p>
          ) : (
            problems.data && <ParcelTable rows={problems.data} now={now} />
          )}
        </section>

        <aside className="space-y-8">
          <section aria-labelledby="shelf-h">
            <h2 id="shelf-h" className="text-lg font-semibold">
              Склад {physical.data && <span className="font-normal text-ink-2">({physical.data.length})</span>}
            </h2>
            <p className="mb-3 text-sm text-ink-2">Що фізично лежить на полицях. У демо замінює ручний сканер.</p>
            {unregistered > 0 && (
              <p className="mb-3 rounded bg-lvl-red-bg px-3 py-2 text-sm text-lvl-red">
                {unregistered === 1
                  ? "1 посилка лежить тут, але система про неї не знає. Відскануйте її, щоб вона не загубилась."
                  : `Посилок, про які система не знає: ${unregistered}. Відскануйте їх, щоб вони не загубились.`}
              </p>
            )}
            {physical.isError ? (
              <p className="text-sm text-lvl-red">Емулятор недоступний. Перевірте, що його запущено.</p>
            ) : physical.isPending ? (
              <Loading />
            ) : (
              <ul className="divide-y divide-line border-y border-line bg-surface">
                {physical.data.length === 0 && <li className="px-3 py-3 text-sm text-ink-2">Полиці порожні.</li>}
                {physical.data.map(({ ttn, registered }) => {
                  const lvl = levelOf.get(ttn);
                  const invisible = !asIs && !registered;
                  return (
                    <li key={ttn} className={`relative flex items-center gap-3 px-3 py-2 ${invisible ? "bg-lvl-red-bg/40" : ""}`}>
                      {invisible && <span className="absolute inset-y-0 left-0 w-1 bg-lvl-red" aria-hidden />}
                      <span className={`h-2.5 w-2.5 shrink-0 rounded-full ${lvl ? levelClass[lvl].rail : "bg-line"}`} title={lvl ? "Є в списку проблемних" : undefined} />
                      <span className="min-w-0">
                        <span className="ttn block">{formatTTN(ttn)}</span>
                        {!asIs && <span className={`block text-xs ${invisible ? "text-lvl-red" : "text-ink-2"}`}>{invisible ? "Немає в системі відділення" : "Зареєстрована"}</span>}
                      </span>
                      <button
                        className={`ml-auto shrink-0 rounded px-3 py-1 text-sm ${invisible ? "bg-brand font-medium text-white hover:bg-brand-dark" : "border border-line hover:bg-bg"}`}
                        onClick={() => setScanning(ttn)}
                      >
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
            {incoming.isPending && <Loading />}
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
            {!asIs && !!incoming.data?.length && <p className="mt-2 text-xs text-ink-2">Час — прогноз за планом маршруту.</p>}
          </section>
        </aside>
      </main>
      <ScanDialog ttn={scanning} mode={clock?.mode} onClose={() => setScanning(null)} />
    </div>
  );
}

function Loading() {
  return <p className="border-y border-line bg-surface px-4 py-6 text-ink-2">Завантаження…</p>;
}
