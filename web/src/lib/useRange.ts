import { useEffect, useState } from "react";
import type { RangeSeconds } from "@/components/Range";

// useRange is the last `seconds` up to now, moved along every refresh
// seconds so that relative ranges keep showing the present.
export function useRange(seconds: RangeSeconds, refresh = 30) {
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000));
  useEffect(() => {
    const t = setInterval(() => setNow(Math.floor(Date.now() / 1000)), refresh * 1000);
    return () => clearInterval(t);
  }, [refresh]);
  return { from: now - seconds, to: now };
}
