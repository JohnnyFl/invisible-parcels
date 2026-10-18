import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { api } from "../lib/api";
import { meQuery, useLocationName, useVirtualNow } from "../lib/queries";

const roleText = { admin: "Симулятор", senior: "Старша зміна", operator: "Оператор" } as const;

export function Header() {
  const { data: me } = useQuery(meQuery);
  const name = useLocationName();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const { now, clock } = useVirtualNow();

  async function logout() {
    await api("/api/logout", { method: "POST" });
    qc.clear();
    navigate({ to: "/login" });
  }

  return (
    <header className="bg-brand text-white">
      <div className="mx-auto flex max-w-7xl flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3">
        <Link to="/" className="text-lg font-semibold">Невидимі посилки</Link>
        <span className="text-sm text-white/85">Концепт для Нової пошти</span>
        <div className="ml-auto flex flex-wrap items-center gap-x-5 gap-y-1 text-sm">
          {clock && (
            <span title="Віртуальний час симуляції">
              {now.toLocaleString("uk-UA", { timeZone: "Europe/Kyiv", day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" })}
              {clock.mode === "as_is" ? ", режим «як зараз»" : ""}
            </span>
          )}
          {me && (
            <span>
              {roleText[me.role]}
              {me.branchId ? `, ${name(me.branchId)}` : ""}
            </span>
          )}
          {me && (
            <button onClick={logout} className="rounded border border-white/60 px-3 py-1 hover:bg-white/10">
              Вийти
            </button>
          )}
        </div>
      </div>
    </header>
  );
}
