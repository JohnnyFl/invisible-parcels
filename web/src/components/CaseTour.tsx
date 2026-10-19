import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../lib/api";

const FOCUS_TTN = "59000000000001";

const post = (path: string, json: unknown = {}) => api(path, { method: "POST", json });
const at = (iso: string) => post("/api/sim/clock", { jumpTo: iso, speed: 0 });
const mode = (m: "as_is" | "with_solution") => post("/api/sim/mode", { mode: m });

type Step = { title: string; text: string; run: () => Promise<unknown> };

/** Scenario 2 told in five steps: each step moves the world, the split view below shows the result. */
const steps: Step[] = [
  {
    title: "Посилку відправлено",
    text: "Посилка їде з Харкова в Київ. Застосунок пише клієнту: «прямує до міста одержувача». Поки що все гаразд.",
    run: async () => {
      await post("/api/sim/scenarios/2");
      await mode("as_is");
      await at("2026-10-17T13:00:00Z");
    },
  },
  {
    title: "Рейс прибув раніше, скани пропущено",
    text: "Авто приїхало у відділення вранці, на кілька годин раніше за план. Посилку виклали на полицю, але не відсканували ні при завантаженні, ні при вивантаженні.",
    run: () => at("2026-10-18T07:00:00Z"),
  },
  {
    title: "Так працює зараз",
    text: "Обіцяні 15:00 минули. Клієнт досі бачить прогноз, а оператор посилки не бачить навіть у надходженнях. Невідому посилку можуть відправити назад, і там вона загубиться.",
    run: () => at("2026-10-18T12:30:00Z"),
  },
  {
    title: "Так з рішенням",
    text: "Система звіряє план маршруту з фактичними сканами: рейс прибув, а посилку не вивантажено. Оператор бачить червону позначку «Розірваний ланцюг» і підказку, де шукати.",
    run: () => mode("with_solution"),
  },
  {
    title: "Оператор знаходить і приймає посилку",
    text: "Оператор сканує посилку з полиці й приймає її. Клієнт одразу бачить «Прибув у відділення». Посилка не загубилась.",
    run: () => post(`/api/sim/accept/${FOCUS_TTN}`),
  },
];

export function CaseTour({ onStart }: { onStart: () => void }) {
  const qc = useQueryClient();
  const [step, setStep] = useState(-1);
  const go = useMutation({
    mutationFn: (i: number) => steps[i].run(),
    onSuccess: (_, i) => {
      setStep(i);
      qc.invalidateQueries();
    },
  });

  if (step < 0) {
    return (
      <div className="flex flex-wrap items-center gap-4 border-y border-line bg-surface px-5 py-4">
        <div className="flex-1">
          <p className="font-medium">Як посилка стає невидимою — і як рішення її знаходить</p>
          <p className="text-sm text-ink-2">П'ять кроків, приблизно дві хвилини. Час і дані змінюються самі.</p>
        </div>
        <button
          className="rounded bg-brand px-5 py-2.5 font-medium text-white hover:bg-brand-dark disabled:opacity-50"
          disabled={go.isPending}
          onClick={() => {
            onStart();
            go.mutate(0);
          }}
        >
          {go.isPending ? "Готуємо…" : "Показати кейс"}
        </button>
      </div>
    );
  }

  const s = steps[step];
  const last = step === steps.length - 1;
  return (
    <div className="border-y border-line bg-surface px-5 py-4" aria-live="polite">
      <div className="mb-3 flex gap-1.5" aria-hidden>
        {steps.map((_, i) => (
          <span key={i} className={`h-1 flex-1 rounded ${i <= step ? "bg-ink" : "bg-line"}`} />
        ))}
      </div>
      <p className="text-sm text-ink-2">Крок {step + 1} з {steps.length}</p>
      <p className="mt-0.5 text-lg font-semibold">{s.title}</p>
      <p className="mt-1 max-w-prose">{s.text}</p>
      {go.isError && <p role="alert" className="mt-2 text-lvl-red">Не вдалося виконати крок: {(go.error as Error).message}</p>}
      <div className="mt-4 flex flex-wrap gap-2">
        {!last && (
          <button className="rounded bg-brand px-5 py-2 font-medium text-white hover:bg-brand-dark disabled:opacity-50" disabled={go.isPending} onClick={() => go.mutate(step + 1)}>
            {go.isPending ? "Зачекайте…" : "Далі"}
          </button>
        )}
        <button className="rounded border border-line px-4 py-2 hover:bg-bg disabled:opacity-50" disabled={go.isPending} onClick={() => (last ? setStep(-1) : go.mutate(0))}>
          {last ? "Завершити" : "Почати спочатку"}
        </button>
      </div>
    </div>
  );
}
