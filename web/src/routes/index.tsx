import { createFileRoute, redirect } from "@tanstack/react-router";
import { homeOf, requireRole } from "../lib/guard";

export const Route = createFileRoute("/")({
  beforeLoad: async ({ context }) => {
    const me = await requireRole(context.queryClient);
    throw redirect({ to: homeOf[me.role] });
  },
});
