import type { TFunction } from "i18next";
import type { Alert, AlertState } from "@/api";

// alertTitle says in words what an alert is about: "Disk almost full",
// not host.disk.
export function alertTitle(t: TFunction, a: Pick<AlertState | Alert, "rule">): string {
  return t(`alert.titles.${a.rule}`, { defaultValue: a.rule });
}

// subjectName is the name of the object an alert is about, the last part of
// its path, and where it is, the rest.
export function subjectName(a: Pick<AlertState | Alert, "subject_path">): { name: string; where: string } {
  const i = a.subject_path.lastIndexOf("/");
  return i < 0 ? { name: a.subject_path, where: "" } : { name: a.subject_path.slice(i + 1), where: a.subject_path.slice(0, i) };
}
