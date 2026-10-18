import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";

/** Subscribes to /api/stream and refetches the query keys the server names. */
export function useInvalidateStream(enabled: boolean) {
  const qc = useQueryClient();
  useEffect(() => {
    if (!enabled) return;
    const es = new EventSource("/api/stream");
    es.onmessage = (ev) => {
      const msg = JSON.parse(ev.data) as { type: string; keys: string[] };
      if (msg.type !== "invalidate") return;
      for (const key of [...msg.keys, "split"]) qc.invalidateQueries({ queryKey: [key] });
    };
    return () => es.close();
  }, [enabled, qc]);
}
