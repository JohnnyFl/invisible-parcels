import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { api, formatTime, formatTTN, type ProblemRow } from "../lib/api";
import { locationsQuery, useLocationName } from "../lib/queries";

export type SeniorAction = { kind: "check" | "confirm"; row: ProblemRow };

/** Under each senior row: the latest check and the actions that close the loop. */
export function SeniorRowActions({ row, onAction }: { row: ProblemRow; onAction: (a: SeniorAction) => void }) {
  const name = useLocationName();
  const risky = row.problems.some((p) => p.kind === "risky_return");
  const c = row.check;
  return (
    <div className="mt-3 flex flex-wrap items-center gap-2">
      {c && (
        <p className={`mr-2 text-sm ${c.status === "not_found" ? "text-lvl-red" : c.status === "found" ? "text-lvl-green" : "text-ink-2"}`}>
          {c.status === "requested" && `Доручено перевірити: ${name(c.branch)}, ${formatTime(c.at)}. Чекаємо відповіді.`}
          {c.status === "found" && `${name(c.branch)}: знайдено, ${formatTime(c.at)}.`}
          {c.status === "not_found" && `${name(c.branch)}: не знайдено, ${formatTime(c.at)}.`}
        </p>
      )}
      <button className="rounded border border-line px-3 py-1 text-sm hover:bg-bg" onClick={() => onAction({ kind: "check", row })}>
        {c ? "Доручити ще раз" : "Доручити перевірку"}
      </button>
      {risky && (
        <button className="rounded border border-line px-3 py-1 text-sm hover:bg-bg" onClick={() => onAction({ kind: "confirm", row })}>
          Підтвердити повернення
        </button>
      )}
    </div>
  );
}

function defaultBranch(row: ProblemRow, branches: string[]): string {
  if (branches.includes(row.location)) return row.location;
  if (branches.includes(row.destination)) return row.destination;
  return branches[0] ?? "";
}

export function SeniorActionDialog({ action, onClose }: { action: SeniorAction | null; onClose: () => void }) {
  const ref = useRef<HTMLDialogElement>(null);
  const qc = useQueryClient();
  const { data: locations } = useQuery(locationsQuery);
  const branches = locations?.filter((l) => l.kind === "branch") ?? [];
  const [branch, setBranch] = useState("");
  const [text, setText] = useState("");

  useEffect(() => {
    if (!action) return;
    setBranch(defaultBranch(action.row, branches.map((b) => b.id)));
    setText("");
    send.reset();
    ref.current?.showModal();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [action]);

  const send = useMutation({
    mutationFn: () =>
      action!.kind === "check"
        ? api(`/api/senior/parcels/${action!.row.ttn}/check`, { method: "POST", json: { branchId: branch, note: text } })
        : api(`/api/senior/parcels/${action!.row.ttn}/confirm-return`, { method: "POST", json: { comment: text } }),
    onSuccess: () => {
      qc.invalidateQueries();
      ref.current?.close();
    },
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    send.mutate();
  };
  const field = "w-full rounded border border-line bg-surface px-3 py-2";

  return (
    <dialog ref={ref} onClose={onClose} className="m-auto w-[min(32rem,calc(100vw-2rem))] rounded border border-line p-0 text-ink">
      {action && (
        <form onSubmit={submit} className="space-y-4 p-5">
          <div>
            <p className="text-sm text-ink-2">{action.kind === "check" ? "Доручити перевірку відділенню" : "Підтвердити повернення"}</p>
            <p className="ttn text-xl font-semibold">{formatTTN(action.row.ttn)}</p>
          </div>
          {action.kind === "check" ? (
            <>
              <label className="block">
                <span className="mb-1 block text-sm text-ink-2">Яке відділення перевіряє</span>
                <select value={branch} onChange={(e) => setBranch(e.target.value)} className={field}>
                  {branches.map((b) => (
                    <option key={b.id} value={b.id}>{b.city}, {b.name}</option>
                  ))}
                </select>
              </label>
              <label className="block">
                <span className="mb-1 block text-sm text-ink-2">Коментар для оператора, необов'язково</span>
                <input value={text} maxLength={200} onChange={(e) => setText(e.target.value)} placeholder="Наприклад: перевірте зону розвантаження" className={field} />
              </label>
              <p className="text-sm text-ink-2">Оператор побачить доручення на своєму екрані й відповість «знайдено» чи «не знайдено».</p>
            </>
          ) : (
            <label className="block">
              <span className="mb-1 block text-sm text-ink-2">Чому повернення обґрунтоване</span>
              <textarea required value={text} maxLength={300} onChange={(e) => setText(e.target.value)} className={field} rows={3} placeholder="Наприклад: посилку пошкоджено, відправник погодив повернення" />
            </label>
          )}
          {send.isError && <p role="alert" className="text-lvl-red">{(send.error as Error).message}</p>}
          <div className="flex gap-2">
            <button className="rounded bg-brand px-4 py-2 font-medium text-white hover:bg-brand-dark disabled:opacity-50" disabled={send.isPending}>
              {action.kind === "check" ? "Доручити" : "Підтвердити"}
            </button>
            <button type="button" className="rounded border border-line px-4 py-2 hover:bg-bg" onClick={() => ref.current?.close()}>Скасувати</button>
          </div>
        </form>
      )}
    </dialog>
  );
}
