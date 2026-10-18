import { formatTime, formatTTN, type Track } from "../lib/api";

/** What the client app shows for a parcel. */
export function TrackCard({ t }: { t: Track }) {
  if (!t.statusCode) return <p className="text-ink-2">Статусу ще немає: посилку не зареєстровано в системі.</p>;
  return (
    <div>
      <p className="ttn text-sm text-ink-2">{formatTTN(t.ttn)}</p>
      <p className="mt-1 text-lg font-medium">{t.status}</p>
      {t.eta && (
        <p className="mt-2">
          Орієнтовно {formatTime(t.eta)}
          {t.isForecast && <span className="ml-2 rounded bg-lvl-grey-bg px-2 py-0.5 text-sm text-lvl-grey">прогноз</span>}
        </p>
      )}
      {t.details && (
        <p className="mt-2 text-ink-2">
          Отримання: {t.details.city}, {t.details.warehouse}
        </p>
      )}
    </div>
  );
}
