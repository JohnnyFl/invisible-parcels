import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api, ApiError, formatTTN, type Mode, type ScanResult } from "../lib/api";
import { useLocationName } from "../lib/queries";

type Action = "accept" | "redirect" | "hold" | "return";

const reasons = [
  { id: "damaged", label: "Пошкоджена" },
  { id: "refused", label: "Відмова отримувача" },
  { id: "other", label: "Інше" },
];

/** Shown after a scan: the system proposes what to do with the parcel. */
export function ScanDialog({ ttn, mode, onClose }: { ttn: string | null; mode?: Mode; onClose: () => void }) {
  const ref = useRef<HTMLDialogElement>(null);
  const qc = useQueryClient();
  const name = useLocationName();
  const [returning, setReturning] = useState(false);
  const [reason, setReason] = useState("damaged");
  const [comment, setComment] = useState("");

  const scan = useMutation({
    mutationFn: (t: string) => api<ScanResult>("/api/operator/scan", { method: "POST", json: { ttn: t } }),
  });
  const act = useMutation({
    mutationFn: (a: Action) =>
      api(`/api/operator/parcels/${ttn}/${a}`, { method: "POST", json: a === "return" ? { reason, comment } : {} }),
    onSuccess: () => {
      qc.invalidateQueries();
      ref.current?.close(); // fires onClose
    },
  });

  useEffect(() => {
    if (!ttn) return;
    setReturning(false);
    setComment("");
    act.reset();
    scan.mutate(ttn);
    ref.current?.showModal();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ttn]);

  const r = scan.data;
  const err = (scan.error ?? act.error) as ApiError | null;
  const primary = "rounded bg-brand px-4 py-2 font-medium text-white hover:bg-brand-dark disabled:opacity-50";
  const secondary = "rounded border border-line px-4 py-2 hover:bg-bg";

  return (
    <dialog ref={ref} onClose={onClose} className="m-auto w-[min(32rem,calc(100vw-2rem))] rounded border border-line p-0 text-ink">
      {ttn && (
        <div className="p-5">
          <p className="text-sm text-ink-2">Скан посилки</p>
          <p className="ttn mb-4 text-2xl font-semibold">{formatTTN(ttn)}</p>

          {scan.isPending && <p>Шукаємо посилку…</p>}
          {err && (
            <p role="alert" className="text-lvl-red">
              {err.status === 429 ? "Перевищено ліміт пошуків за годину. Зверніться до старшої зміни." : err.message}
            </p>
          )}

          {r && !returning && (
            <>
              {r.outcome === "accept" && <p className="mb-4">Посилка призначена вашому відділенню.</p>}
              {r.outcome === "redirect" && (
                <p className="mb-4">Посилка для іншого відділення. Наступна точка маршруту: <strong>{name(r.nextLocation)}</strong>.</p>
              )}
              {r.outcome === "hold" && <p className="mb-4">Посилки немає в системі. Покладіть її на полицю ідентифікації: старша зміна побачить її у своєму списку.</p>}
              {r.outcome === "not_found" && <p className="mb-4">Посилку не знайдено.</p>}
              <div className="flex flex-wrap gap-2">
                {r.outcome === "accept" && <button className={primary} disabled={act.isPending} onClick={() => act.mutate("accept")}>Прийняти</button>}
                {r.outcome === "redirect" && <button className={primary} disabled={act.isPending} onClick={() => act.mutate("redirect")}>Переадресувати</button>}
                {r.outcome === "hold" && <button className={primary} disabled={act.isPending} onClick={() => act.mutate("hold")}>На ідентифікацію</button>}
                {r.outcome === "not_found" && <button className={primary} disabled={act.isPending} onClick={() => act.mutate("return")}>Повернути</button>}
                {(r.outcome === "hold" || r.outcome === "redirect") && (
                  <button className={secondary} onClick={() => setReturning(true)}>Повернути…</button>
                )}
                <button className={secondary} onClick={() => ref.current?.close()}>Закрити</button>
              </div>
            </>
          )}

          {returning && (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                act.mutate("return");
              }}
            >
              <fieldset className="mb-4">
                <legend className="mb-2">Чому повертаєте посилку?</legend>
                {reasons.map((x) => (
                  <label key={x.id} className="flex items-center gap-2 py-1">
                    <input type="radio" name="reason" checked={reason === x.id} onChange={() => setReason(x.id)} className="accent-ink" />
                    {x.label}
                  </label>
                ))}
                {reason === "other" && (
                  <textarea required value={comment} onChange={(e) => setComment(e.target.value)} className="mt-2 w-full rounded border border-line p-2" placeholder="Опишіть причину" />
                )}
              </fieldset>
              {mode === "with_solution" && <p className="mb-4 text-sm text-ink-2">Повернення побачить старша зміна, доки посилку не відсканують деінде.</p>}
              <div className="flex gap-2">
                <button className={primary} disabled={act.isPending}>Повернути</button>
                <button type="button" className={secondary} onClick={() => setReturning(false)}>Назад</button>
              </div>
            </form>
          )}
        </div>
      )}
    </dialog>
  );
}
