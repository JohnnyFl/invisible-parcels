import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api, formatTTN } from "../lib/api";
import { useLocationName } from "../lib/queries";

type Kind = "normal" | "not_loaded" | "missed_scans" | "misrouted" | "unknown" | "incident";
interface Added { ttn: string; kind: Kind; lookAt: string }

const kindText: Record<Kind, string> = {
  normal: "Звичайна, доїде без проблем",
  not_loaded: "Не завантажили на рейс",
  missed_scans: "Скани пропущено",
  misrouted: "Не в те відділення",
  unknown: "Невідома: про неї немає даних",
  incident: "Інцидент на рейсі",
};

/** Adds random parcels to the running world; starts time if it is paused. */
export function RandomParcels({ onPick }: { onPick: (ttn: string) => void }) {
  const qc = useQueryClient();
  const name = useLocationName();
  const [added, setAdded] = useState<Added[]>([]);
  const add = useMutation({
    mutationFn: () => api<Added[]>("/api/sim/parcels/random", { method: "POST", json: { count: 5 } }),
    onSuccess: (list) => {
      setAdded((prev) => [...list, ...prev]);
      qc.invalidateQueries();
    },
  });

  return (
    <section aria-labelledby="random-h">
      <div className="mb-3 flex flex-wrap items-end gap-4">
        <div className="flex-1">
          <h2 id="random-h" className="text-xl font-semibold">Живий потік посилок</h2>
          <p className="text-sm text-ink-2">
            Кожне натискання додає 5 посилок до поточного світу з випадковою долею, без скидання сценарію. Якщо час стоїть, він запуститься. Посилки їдуть близько доби віртуального часу: щоб побачити результат швидше, натисніть ×600 або +6 год.
          </p>
        </div>
        <button className="rounded bg-ink px-4 py-2 font-medium text-white hover:bg-ink/85 disabled:opacity-50" disabled={add.isPending} onClick={() => add.mutate()}>
          {add.isPending ? "Додаємо…" : "Додати 5 випадкових посилок"}
        </button>
      </div>
      {add.isError && <p role="alert" className="mb-2 text-lvl-red">Не вдалося додати: {(add.error as Error).message}. Перевірте, що емулятор запущено.</p>}
      {added.length > 0 && (
        <ul className="divide-y divide-line border-y border-line bg-surface text-sm">
          {added.map((a) => (
            <li key={a.ttn} className="flex flex-wrap items-center gap-x-6 gap-y-1 px-4 py-2">
              <button className="ttn font-medium underline decoration-line underline-offset-4 hover:decoration-ink" onClick={() => onPick(a.ttn)} title="Показати в «Одна посилка, два погляди»">
                {formatTTN(a.ttn)}
              </button>
              <span className={a.kind === "normal" ? "text-ink-2" : ""}>{kindText[a.kind]}</span>
              <span className="text-ink-2 sm:ml-auto">Дивіться: оператор, {name(a.lookAt)}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
