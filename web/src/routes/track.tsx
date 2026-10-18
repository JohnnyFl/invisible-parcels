import { createFileRoute, Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { TrackCard } from "../components/TrackCard";
import { api, ApiError, type Track } from "../lib/api";

export const Route = createFileRoute("/track")({ component: TrackPage });

function TrackPage() {
  const [ttn, setTtn] = useState("");
  const [phone, setPhone] = useState("");
  const [query, setQuery] = useState<{ ttn: string; phone: string } | null>(null);
  const { data, error, isFetching } = useQuery({
    queryKey: ["track", query?.ttn, query?.phone],
    queryFn: () => api<Track>(`/api/track/${query!.ttn}?phone=${encodeURIComponent(query!.phone)}`),
    enabled: !!query,
    retry: false,
    refetchInterval: 5000,
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    setQuery({ ttn: ttn.replace(/\D/g, ""), phone });
  }

  const status = (error as ApiError | null)?.status;
  return (
    <div className="min-h-screen">
      <div className="bg-brand px-4 py-3 text-white">
        <span className="text-lg font-semibold">Невидимі посилки</span>
        <span className="ml-4 text-sm text-white/85">Концепт для Нової пошти</span>
      </div>
      <main className="mx-auto max-w-md px-4 py-10">
        <h1 className="text-2xl font-semibold">Відстежити посилку</h1>
        <p className="mt-1 text-ink-2">Так статус бачить клієнт у застосунку.</p>
        <form onSubmit={submit} className="mt-6 space-y-4">
          <label className="block">
            <span className="mb-1 block text-sm text-ink-2">Номер накладної</span>
            <input required inputMode="numeric" value={ttn} onChange={(e) => setTtn(e.target.value)} placeholder="59 0000 0000 0001" className="ttn w-full rounded border border-line bg-surface px-3 py-2" />
          </label>
          <label className="block">
            <span className="mb-1 block text-sm text-ink-2">Телефон, необов'язково: покаже відділення отримання</span>
            <input type="tel" value={phone} onChange={(e) => setPhone(e.target.value)} placeholder="067 000 0001" className="w-full rounded border border-line bg-surface px-3 py-2" />
          </label>
          <button className="w-full rounded bg-brand px-4 py-2.5 font-medium text-white hover:bg-brand-dark">Відстежити</button>
        </form>
        <div className="mt-8" aria-live="polite">
          {isFetching && !data && <p className="text-ink-2">Шукаємо…</p>}
          {status === 404 && <p>Посилку не знайдено. Перевірте номер: 14 цифр, починається з 59.</p>}
          {status === 400 && <p>Номер має складатися з 14 цифр і починатися з 59.</p>}
          {data && (
            <div className="border-y border-line bg-surface px-4 py-4">
              <TrackCard t={data} />
            </div>
          )}
        </div>
        <p className="mt-8 text-sm text-ink-2">
          Працюєте у відділенні? <Link to="/login" className="text-ink underline">Увійти</Link>
        </p>
      </main>
    </div>
  );
}
