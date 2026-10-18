import { createFileRoute } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Header } from "../components/Header";
import { LevelBadge, problemText } from "../components/LevelBadge";
import { TrackCard } from "../components/TrackCard";
import { api, formatTime, type ClockState, type Mode, type Split } from "../lib/api";
import { requireRole } from "../lib/guard";
import { useLocationName, useVirtualNow } from "../lib/queries";
import { useInvalidateStream } from "../lib/useInvalidateStream";

export const Route = createFileRoute("/sim")({
  beforeLoad: ({ context }) => requireRole(context.queryClient, "admin"),
  component: Sim,
});

const FOCUS_TTN = "59000000000001";

const scenarios = [
  { id: 1, title: "Посилку не завантажили на рейс", text: "Вона лишилась на сортувальному центрі, а рейс поїхав без неї. Клієнт бачить «у місті одержувача».", branch: "Київ, Відділення №12" },
  { id: 2, title: "Скани завантаження й вивантаження пропущено", text: "Кейс із документа: посилка лежить у відділенні, а система її не бачить.", branch: "Київ, Відділення №12" },
  { id: 3, title: "Посилку привезли не в те відділення", text: "Її вивантажили у Львові замість Києва.", branch: "Львів, Відділення №5" },
  { id: 4, title: "Невідома посилка у відділенні", text: "Про неї немає жодних даних. Сьогодні таку посилку повертають навмання.", branch: "Харків, Відділення №3" },
  { id: 5, title: "Інцидент на маршруті", text: "Рейс постраждав від обстрілу. Посилки мають відому причину затримки, а не «загубились».", branch: "Київ, Відділення №12" },
];

const speeds = [
  { label: "Пауза", speed: 0 },
  { label: "×60", speed: 60 },
  { label: "×600", speed: 600 },
];

function Sim() {
  useInvalidateStream(true);
  const qc = useQueryClient();
  const name = useLocationName();
  const { now, clock } = useVirtualNow();
  const [ttn, setTtn] = useState(FOCUS_TTN);
  const [launched, setLaunched] = useState<number | null>(null);
  const split = useQuery({ queryKey: ["split", ttn], queryFn: () => api<Split>(`/api/sim/split/${ttn}`), enabled: /^59\d{12}$/.test(ttn) });

  const post = useMutation({
    mutationFn: ({ path, json }: { path: string; json?: unknown }) => api<ClockState>(path, { method: "POST", json: json ?? {} }),
    onSuccess: (c) => {
      qc.setQueryData(["clock"], c);
      qc.invalidateQueries();
    },
  });
  const setClock = (json: unknown) => post.mutate({ path: "/api/sim/clock", json });
  const jump = (hours: number) => setClock({ jumpTo: new Date(now.getTime() + hours * 3600_000).toISOString() });
  const setMode = (mode: Mode) => post.mutate({ path: "/api/sim/mode", json: { mode } });

  const seg = (active: boolean) => `px-4 py-2 ${active ? "bg-ink text-white" : "bg-surface hover:bg-bg"}`;

  return (
    <div className="min-h-screen">
      <Header />
      <main className="mx-auto max-w-7xl space-y-10 px-4 py-6">
        {post.isError && <p role="alert" className="text-lvl-red">{(post.error as Error).message}. Перевірте, що емулятор запущено.</p>}

        <section className="flex flex-wrap items-end gap-x-10 gap-y-6">
          <div>
            <p className="text-sm text-ink-2">Віртуальний час, Київ</p>
            <p className="text-4xl font-semibold tabular-nums">
              {now.toLocaleString("uk-UA", { timeZone: "Europe/Kyiv", weekday: "short", day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" })}
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <div className="inline-flex overflow-hidden rounded border border-line" role="group" aria-label="Швидкість часу">
              {speeds.map((s) => (
                <button key={s.label} aria-pressed={clock?.speed === s.speed} className={seg(clock?.speed === s.speed)} onClick={() => setClock({ speed: s.speed })}>
                  {s.label}
                </button>
              ))}
            </div>
            <button className="rounded border border-line bg-surface px-4 py-2 hover:bg-bg" onClick={() => jump(1)}>+1 год</button>
            <button className="rounded border border-line bg-surface px-4 py-2 hover:bg-bg" onClick={() => jump(6)}>+6 год</button>
          </div>
          <div className="inline-flex overflow-hidden rounded border border-line" role="group" aria-label="Режим показу">
            <button aria-pressed={clock?.mode === "as_is"} className={seg(clock?.mode === "as_is")} onClick={() => setMode("as_is")}>Як зараз</button>
            <button aria-pressed={clock?.mode === "with_solution"} className={seg(clock?.mode === "with_solution")} onClick={() => setMode("with_solution")}>З рішенням</button>
          </div>
        </section>

        <section aria-labelledby="split-h">
          <div className="mb-3 flex flex-wrap items-end gap-4">
            <h1 id="split-h" className="text-xl font-semibold">Одна посилка, два погляди</h1>
            <label className="text-sm">
              <span className="mr-2 text-ink-2">Накладна</span>
              <input value={ttn} onChange={(e) => setTtn(e.target.value.replace(/\D/g, ""))} className="ttn w-44 rounded border border-line bg-surface px-2 py-1" />
            </label>
          </div>
          {split.data && <Verdict split={split.data} />}
          <div className="grid border-y border-line bg-surface md:grid-cols-2 md:divide-x md:divide-line">
            <div className="p-5">
              <h2 className="mb-3 font-medium text-ink-2">Що бачить клієнт</h2>
              {split.data && <TrackCard t={split.data.client} />}
            </div>
            <div className="border-t border-line p-5 md:border-t-0">
              <h2 className="mb-3 font-medium text-ink-2">Що бачить оператор{split.data?.operator.branch ? `: ${name(split.data.operator.branch)}` : ""}</h2>
              {split.data && (
                <div className="space-y-3">
                  <p className="text-lg">
                    У списку надходжень: <strong>{split.data.operator.visibleInIncoming ? "так" : "ні"}</strong>
                  </p>
                  {split.data.mode === "as_is" ? (
                    <p className="text-ink-2">Списку проблемних посилок у системі немає.</p>
                  ) : (
                    <div className="flex flex-wrap items-center gap-3">
                      <LevelBadge level={split.data.operator.level} />
                      <span>{split.data.operator.problems.map((p) => problemText[p.kind]).join(", ")}</span>
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>
        </section>

        <section aria-labelledby="sc-h">
          <h2 id="sc-h" className="mb-1 text-xl font-semibold">Сценарії</h2>
          <p className="mb-3 text-sm text-ink-2">Запуск очищає дані й перемотує час на 17.10, 11:00. Час іде зі швидкістю ×60. Посилка в центрі сценарію: 59 0000 0000 0001.</p>
          <ul className="divide-y divide-line border-y border-line bg-surface">
            {scenarios.map((s) => (
              <li key={s.id} className={`relative flex flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3 ${launched === s.id ? "bg-bg" : ""}`}>
                {launched === s.id && <span className="absolute inset-y-0 left-0 w-1 bg-ink" aria-hidden />}
                <div className="min-w-64 flex-1">
                  <p className="font-medium">
                    {s.title}
                    {launched === s.id && <span className="ml-2 rounded bg-ink px-2 py-0.5 text-xs font-normal text-white">Запущено</span>}
                  </p>
                  <p className="text-sm text-ink-2">{s.text}</p>
                  <p className="text-sm text-ink-2">Дивіться: оператор, {s.branch}</p>
                </div>
                <button
                  className="rounded bg-brand px-4 py-2 font-medium text-white hover:bg-brand-dark disabled:opacity-50"
                  disabled={post.isPending}
                  onClick={() => {
                    setTtn(FOCUS_TTN);
                    setLaunched(s.id);
                    post.mutate({ path: `/api/sim/scenarios/${s.id}` });
                  }}
                >
                  {launched === s.id ? "Запустити знову" : "Запустити"}
                </button>
              </li>
            ))}
          </ul>
        </section>
      </main>
    </div>
  );
}

/** One sentence that says whether the two views disagree — the point of the demo. */
function Verdict({ split }: { split: Split }) {
  const { client, operator, mode } = split;
  const promised = client.statusCode === "5" || client.statusCode === "6";
  const seen = operator.visibleInIncoming || operator.level !== "green";
  let tone = "bg-lvl-green-bg text-lvl-green";
  let text = "Клієнт і оператор бачать узгоджену картину.";
  if (!client.statusCode && !seen) {
    tone = "bg-lvl-grey-bg text-lvl-grey";
    text = "Ні клієнт, ні оператор поки нічого не бачать про цю посилку.";
  } else if (promised && !seen) {
    tone = "bg-lvl-red-bg text-lvl-red";
    text = `Розбіжність: клієнту обіцяно ${client.eta ? formatTime(client.eta) : "доставку"}, а оператор відділення посилки не бачить.`;
  } else if (mode === "with_solution" && operator.level !== "green") {
    tone = "bg-lvl-orange-bg text-lvl-orange";
    text = `Рішення працює: оператор бачить проблему «${operator.problems.map((p) => problemText[p.kind]).join(", ")}» і знає, що робити.`;
  }
  return <p className={`mb-3 rounded px-4 py-3 font-medium ${tone}`} role="status">{text}</p>;
}
