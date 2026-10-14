import type { Level } from "@/lib/status";
import { levelColor } from "@/lib/status";

// StatusIcon says how something stands by shape as well as color: a ring
// with a check for OK, a half-filled ring for WARN, a filled disc with a
// mark for CRIT, a dashed ring for UNKNOWN.
export function StatusIcon({ level, size = 14, className = "" }: { level: Level; size?: number; className?: string }) {
  const c = levelColor[level];
  return (
    <svg viewBox="0 0 14 14" width={size} height={size} className={`shrink-0 ${className}`} aria-hidden>
      {level === "ok" && (
        <>
          <circle cx="7" cy="7" r="5.75" fill="none" stroke={c} strokeWidth="1.5" />
          <path d="M4.6 7.1l1.6 1.6 3.2-3.3" fill="none" stroke={c} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
        </>
      )}
      {level === "warn" && (
        <>
          <circle cx="7" cy="7" r="5.75" fill="none" stroke={c} strokeWidth="1.5" />
          <path d="M7 3.25a3.75 3.75 0 0 1 0 7.5z" fill={c} />
        </>
      )}
      {level === "crit" && (
        <>
          <circle cx="7" cy="7" r="6.5" fill={c} />
          <path d="M7 3.9v3.6" stroke="white" strokeWidth="1.6" strokeLinecap="round" />
          <circle cx="7" cy="9.9" r="0.95" fill="white" />
        </>
      )}
      {level === "unknown" && <circle cx="7" cy="7" r="5.75" fill="none" stroke={c} strokeWidth="1.5" strokeDasharray="2.2 2.2" />}
    </svg>
  );
}

// Dot is a status icon in a line of text.
export function Dot({ level, className = "" }: { level: Level; className?: string }) {
  return <StatusIcon level={level} className={className} />;
}
