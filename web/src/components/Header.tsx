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
      <div className="mx-auto flex max-w-7xl flex-wrap items-center gap-x-6 gap-y-1 px-4 py-2.5 sm:py-3">
        <Link to="/" className="text-lg font-semibold">Невидимі посилки</Link>
        <span className="hidden text-sm text-white/85 md:inline">Концепт для Нової пошти</span>
        {me && (
          <button onClick={logout} className="order-2 ml-auto rounded border border-white/60 px-3 py-1 text-sm hover:bg-white/10 sm:order-3 sm:ml-0">
            Вийти
          </button>
        )}
        <div className="order-3 flex w-full flex-wrap gap-x-4 text-sm text-white/90 sm:order-2 sm:ml-auto sm:w-auto">
          {me && (
            <span>
              {roleText[me.role]}
              {me.branchId ? `, ${name(me.branchId)}` : ""}
            </span>
          )}
          {clock && (
            <span title="Віртуальний час симуляції">
              {now.toLocaleString("uk-UA", { timeZone: "Europe/Kyiv", day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" })}
              {clock.mode === "as_is" ? ", режим «як зараз»" : ""}
            </span>
          )}
        </div>
      </div>
    </header>
  );
}
