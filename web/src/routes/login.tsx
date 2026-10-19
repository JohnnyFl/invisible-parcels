import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { api, ApiError, type Me, type Role } from "../lib/api";
import { homeOf } from "../lib/guard";
import { locationsQuery } from "../lib/queries";

export const Route = createFileRoute("/login")({ component: Login });

const roles: { id: Role; label: string; hint: string }[] = [
  { id: "operator", label: "Оператор", hint: "Посилки свого відділення" },
  { id: "senior", label: "Старша зміна", hint: "Усі відділення, ризикові повернення" },
  { id: "admin", label: "Симулятор", hint: "Сценарії, час, режим показу" },
];

function Login() {
  const { data: locations } = useQuery(locationsQuery);
  const branches = locations?.filter((l) => l.kind === "branch") ?? [];
  const [role, setRole] = useState<Role>("operator");
  const [branchId, setBranchId] = useState("branch-kyiv-12");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const qc = useQueryClient();
  const navigate = useNavigate();

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    try {
      const me = await api<Me>("/api/login", { method: "POST", json: { role, branchId, password } });
      qc.setQueryData(["me"], me);
      navigate({ to: homeOf[me.role] });
    } catch (err) {
      const status = err instanceof ApiError ? err.status : 0;
      setError(
        status === 401
          ? "Невірний пароль. Перевірте роль і спробуйте ще раз."
          : status === 429
            ? "Забагато невдалих спроб. Спробуйте за 15 хвилин."
            : "Не вдалося увійти. Перевірте, що сервер запущено.",
      );
    }
  }

  return (
    <div className="min-h-screen">
      <div className="bg-brand px-4 py-3 text-white">
        <span className="text-lg font-semibold">Невидимі посилки</span>
        <span className="ml-4 text-sm text-white/85">Концепт для Нової пошти</span>
      </div>
      <main className="mx-auto max-w-md px-4 py-10">
        <h1 className="text-2xl font-semibold">Вхід</h1>
        <form onSubmit={submit} className="mt-6 space-y-6">
          <fieldset>
            <legend className="mb-2 text-sm text-ink-2">Роль</legend>
            <div className="grid gap-2">
              {roles.map((r) => (
                <label key={r.id} className={`flex cursor-pointer items-start gap-3 rounded border px-3 py-2 ${role === r.id ? "border-ink bg-surface" : "border-line"}`}>
                  <input type="radio" name="role" value={r.id} checked={role === r.id} onChange={() => setRole(r.id)} className="mt-1 accent-ink" />
                  <span>
                    <span className="block font-medium">{r.label}</span>
                    <span className="block text-sm text-ink-2">{r.hint}</span>
                  </span>
                </label>
              ))}
            </div>
          </fieldset>
          {role === "operator" && (
            <label className="block">
              <span className="mb-1 block text-sm text-ink-2">Відділення</span>
              <select value={branchId} onChange={(e) => setBranchId(e.target.value)} className="w-full rounded border border-line bg-surface px-3 py-2">
                {branches.map((b) => (
                  <option key={b.id} value={b.id}>{b.city}, {b.name}</option>
                ))}
              </select>
            </label>
          )}
          <label className="block">
            <span className="mb-1 block text-sm text-ink-2">Пароль</span>
            <input type="password" required value={password} onChange={(e) => setPassword(e.target.value)} className="w-full rounded border border-line bg-surface px-3 py-2" autoComplete="current-password" />
          </label>
          {error && <p role="alert" className="text-sm text-lvl-red">{error}</p>}
          <button type="submit" className="w-full rounded bg-brand px-4 py-2.5 font-medium text-white hover:bg-brand-dark">Увійти</button>
        </form>
        <p className="mt-8 text-sm text-ink-2">
          Ви отримувач чи відправник? <Link to="/track" className="text-ink underline">Відстежити посилку</Link>
        </p>
      </main>
    </div>
  );
}
