import { useQuery } from "@tanstack/react-query";
import { getMe } from "@/api";

// useMe is the logged-in user; the gate has already loaded it. An admin may
// change what they see; an admin of the whole hub also what belongs to it,
// such as users, tokens and probes. A user kept to a customer, project or
// environment sees only that (decision 0023).
export function useMe() {
  const me = useQuery({ queryKey: ["me"], queryFn: getMe, retry: false });
  const scoped = me.data !== undefined && me.data.scope_id !== 1;
  const isAdmin = me.data?.role === "admin";
  return { user: me.data, isAdmin, isHubAdmin: isAdmin && !scoped, scoped };
}
