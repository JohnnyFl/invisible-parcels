import { queryOptions, useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api, type ClockState, type Location, type Me } from "./api";

export const meQuery = queryOptions({ queryKey: ["me"], queryFn: () => api<Me>("/api/me"), retry: false, staleTime: Infinity });
export const locationsQuery = queryOptions({ queryKey: ["locations"], queryFn: () => api<Location[]>("/api/locations"), staleTime: Infinity });
export const clockQuery = queryOptions({ queryKey: ["clock"], queryFn: () => api<ClockState>("/api/clock") });

export function useLocationName() {
  const { data } = useQuery(locationsQuery);
  return (id?: string) => {
    const l = data?.find((x) => x.id === id);
    // hubs already carry the city in their name: "Сортувальний центр Київ"
    if (!l) return id ?? "";
    return l.kind === "hub" ? l.name : `${l.city}, ${l.name}`;
  };
}

/** Virtual "now" that keeps ticking locally between server updates. */
export function useVirtualNow(): { now: Date; clock?: ClockState } {
  const { data: clock, dataUpdatedAt } = useQuery(clockQuery);
  const [tick, setTick] = useState(Date.now());
  useEffect(() => {
    const id = setInterval(() => setTick(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);
  if (!clock) return { now: new Date() };
  const base = new Date(clock.virtualNow).getTime();
  return { now: new Date(base + Math.max(0, tick - dataUpdatedAt) * clock.speed), clock };
}
