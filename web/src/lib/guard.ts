import { redirect } from "@tanstack/react-router";
import type { QueryClient } from "@tanstack/react-query";
import { meQuery } from "./queries";
import type { Me, Role } from "./api";

export const homeOf: Record<Role, "/operator" | "/senior" | "/sim"> = { operator: "/operator", senior: "/senior", admin: "/sim" };

/** beforeLoad helper: the session must exist and have the role. */
export async function requireRole(queryClient: QueryClient, role?: Role): Promise<Me> {
  let me: Me;
  try {
    me = await queryClient.fetchQuery(meQuery);
  } catch {
    throw redirect({ to: "/login" });
  }
  if (role && me.role !== role) throw redirect({ to: homeOf[me.role] });
  return me;
}
