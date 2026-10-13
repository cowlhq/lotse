import { useQuery } from "@tanstack/react-query";
import { getUpdates } from "@/api";
import { newer } from "@/lib/version";

// useUpdates is what the hub knows about releases; updateFor says whether
// an agent of that version has a newer one waiting.
export function useUpdates() {
  const updates = useQuery({ queryKey: ["updates"], queryFn: getUpdates, staleTime: 60_000 });
  const latest = updates.data?.latest?.version;
  return { updates: updates.data, latest, updateFor: (v: string) => (newer(latest, v) ? latest : undefined) };
}
