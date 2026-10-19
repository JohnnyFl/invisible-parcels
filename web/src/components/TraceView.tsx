import { useQuery } from "@tanstack/react-query";
import { api, ApiError, formatTime, formatTTN, type LegState, type Trace, type TraceLeg } from "../lib/api";
import { useLocationName } from "../lib/queries";

const kindText: Record<string, string> = {
  accepted: "прийнято",
  loaded: "завантажено на рейс",
  arrived_hub: "прибула на сортувальний центр",
  sorted: "відсортовано",
  unloaded: "вивантажено",
  arrived_branch: "прибула у відділення",
  operator_scan: "скан оператора",
  hold: "на ідентифікації",
  redirected: "переадресовано",
};

const stateStyle: Record<LegState, { rail: string; label: string; text: string }> = {
  done: { rail: "bg-lvl-green", label: "Підтверджено", text: "text-lvl-green" },
  in_transit: { rail: "bg-lvl-grey", label: "У дорозі", text: "text-lvl-grey" },
  broken: { rail: "bg-lvl-red", label: "Слід обірвався", text: "text-lvl-red" },
  incident: { rail: "bg-lvl-grey", label: "Інцидент", text: "text-lvl-grey" },
  pending: { rail: "bg-line", label: "Ще не почалось", text: "text-ink-2" },
};

/** One scan in a leg: ✓ with time when confirmed, — when not. */
function Check({ label, at }: { label: string; at?: string }) {
  return (
    <li className={at ? "" : "text-ink-2"}>
      <span aria-hidden className="mr-1.5 inline-block w-3">{at ? "✓" : "—"}</span>
      {label}
      {at ? `, ${formatTime(at)}` : ", не підтверджено"}
    </li>
  );
}

function Leg({ leg }: { leg: TraceLeg }) {
  const name = useLocationName();
  const st = stateStyle[leg.state];
  return (
    <li className="relative pb-4 pl-6 last:pb-0">
      <span className={`absolute bottom-0 left-1.5 top-1.5 w-1 rounded ${st.rail}`} aria-hidden />
      <p className="font-medium">
        {name(leg.from)} → {name(leg.to)}
      </p>
      <p className={`text-sm font-medium ${st.text}`}>
        {st.label}
        <span className="font-normal text-ink-2">, за планом до {formatTime(leg.plannedArrival)}</span>
      </p>
      {leg.state !== "pending" && (
        <ul className="mt-1 text-sm">
          <Check label="Завантаження" at={leg.loaded} />
          <Check label="Рейс виїхав" at={leg.departed} />
          <Check label="Рейс прибув" at={leg.arrived} />
          <Check label="Вивантаження" at={leg.unloaded} />
        </ul>
      )}
    </li>
  );
}

/** "Where the chain broke": the planned route next to the scans that actually happened. */
export function TraceView({ ttn, base }: { ttn: string; base: "/api/senior/trace" | "/api/operator/trace" }) {
  const name = useLocationName();
  const { data, error, isPending } = useQuery({
    queryKey: ["trace", base, ttn],
    queryFn: () => api<Trace>(`${base}/${ttn}`),
    retry: false,
  });
  if (isPending) return <p className="text-ink-2">Шукаємо слід…</p>;
  const status = (error as ApiError | null)?.status;
  if (status === 404) return <p>Посилку не знайдено{base === "/api/operator/trace" ? " серед посилок вашого відділення" : ""}.</p>;
  if (status === 400) return <p>Номер має складатися з 14 цифр і починатися з 59.</p>;
  if (error || !data) return <p className="text-lvl-red">Не вдалося отримати слід.</p>;

  return (
    <div>
      <p className="ttn text-xl font-semibold">{formatTTN(data.ttn)}</p>
      {data.destination && <p className="text-sm text-ink-2">Призначення: {name(data.destination)}</p>}

      {data.break ? (
        <p className="mt-3 rounded bg-lvl-red-bg px-3 py-2 text-lvl-red" role="status">{data.break.hint}</p>
      ) : data.delivered ? (
        <p className="mt-3 rounded bg-lvl-green-bg px-3 py-2 text-lvl-green" role="status">Посилка дійшла до відділення призначення.</p>
      ) : null}

      {data.lastConfirmed && (
        <p className="mt-3 text-sm">
          <span className="text-ink-2">Останній підтверджений скан: </span>
          {name(data.lastConfirmed.location)}, {kindText[data.lastConfirmed.kind] ?? data.lastConfirmed.kind}, {formatTime(data.lastConfirmed.at)}
        </p>
      )}

      {data.legs.length === 0 ? (
        <p className="mt-3 text-ink-2">Маршруту немає: посилки немає в системі, відомий лише скан у відділенні.</p>
      ) : (
        <ol className="mt-4">
          {data.legs.map((l) => (
            <Leg key={l.seq} leg={l} />
          ))}
        </ol>
      )}
    </div>
  );
}
