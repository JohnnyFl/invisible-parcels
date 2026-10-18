import { formatAgo, formatTTN, type ProblemRow } from "../lib/api";
import { useLocationName } from "../lib/queries";
import { LevelBadge, levelClass, problemText } from "./LevelBadge";

/** Problem list: a coloured rail per row, worst first (server sorts). */
export function ParcelTable({ rows, now, showBranch }: { rows: ProblemRow[]; now: Date; showBranch?: boolean }) {
  const name = useLocationName();
  return (
    <ul className="divide-y divide-line border-y border-line bg-surface">
      {rows.map((r) => {
        const since = r.problems.reduce((a, p) => (p.since < a ? p.since : a), r.problems[0].since);
        return (
          <li key={r.ttn} className="relative flex flex-wrap items-center gap-x-6 gap-y-2 py-3 pl-5 pr-3">
            <span className={`absolute inset-y-0 left-0 w-1.5 ${levelClass[r.level].rail}`} aria-hidden />
            <div className="w-36 shrink-0">
              <LevelBadge level={r.level} />
            </div>
            <div className="min-w-44">
              <div className="ttn text-lg font-semibold">{formatTTN(r.ttn)}</div>
              <div className="text-sm text-ink-2">{r.problems.map((p) => problemText[p.kind]).join(", ")}</div>
            </div>
            <div className="text-sm">
              <div>{formatAgo(since, now)}</div>
              <div className="text-ink-2">триває</div>
            </div>
            <div className="text-sm">
              <div>{r.location ? name(r.location) : "Невідомо"}</div>
              <div className="text-ink-2">{showBranch && r.destination ? `Призначення: ${name(r.destination)}` : "Остання відома точка"}</div>
            </div>
          </li>
        );
      })}
    </ul>
  );
}
