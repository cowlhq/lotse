import { useQuery } from "@tanstack/react-query";
import { getMe } from "@/api";

// useMe is the logged-in user; the gate has already loaded it.
export function useMe() {
  const me = useQuery({ queryKey: ["me"], queryFn: getMe, retry: false });
  return { user: me.data, isAdmin: me.data?.role === "admin" };
}
