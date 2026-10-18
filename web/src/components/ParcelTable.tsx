import { formatAgo, formatTTN, type ProblemRow } from "../lib/api";
import { useLocationName } from "../lib/queries";
import { LevelBadge, levelClass, problemHint, problemText } from "./LevelBadge";

/** Problem list: a coloured rail per row, worst first (server sorts), with what to do. */
export function ParcelTable({ rows, now, showBranch }: { rows: ProblemRow[]; now: Date; showBranch?: boolean }) {
  const name = useLocationName();
  return (
    <ul className="divide-y divide-line border-y border-line bg-surface">
      {rows.map((r) => {
        const since = r.problems.reduce((a, p) => (p.since < a ? p.since : a), r.problems[0].since);
        const worst = r.problems.reduce((a, p) => (order[p.severity] > order[a.severity] ? p : a), r.problems[0]);
        return (
          <li key={r.ttn} className="relative py-3.5 pl-5 pr-4">
            <span className={`absolute inset-y-0 left-0 w-1.5 ${levelClass[r.level].rail}`} aria-hidden />
            <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
              <span className="ttn text-lg font-semibold">{formatTTN(r.ttn)}</span>
              <LevelBadge level={r.level} />
              <span className="text-sm text-ink-2">{r.problems.map((p) => problemText[p.kind]).join(", ")}</span>
            </div>
            <p className="mt-1.5 max-w-prose">{problemHint[worst.kind]}</p>
            <dl className="mt-2 flex flex-wrap gap-x-6 gap-y-1 text-sm">
              <div className="flex gap-1.5">
                <dt className="text-ink-2">Триває</dt>
                <dd>{formatAgo(since, now)}</dd>
              </div>
              <div className="flex gap-1.5">
                <dt className="text-ink-2">Остання точка</dt>
                <dd>{r.location ? name(r.location) : "немає даних"}</dd>
              </div>
              {showBranch && r.destination && (
                <div className="flex gap-1.5">
                  <dt className="text-ink-2">Призначення</dt>
                  <dd>{name(r.destination)}</dd>
                </div>
              )}
            </dl>
          </li>
        );
      })}
    </ul>
  );
}

const order = { green: 0, grey: 1, yellow: 2, orange: 3, red: 4 } as const;
