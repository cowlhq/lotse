// newer says whether version a is newer than b, as the hub sees it:
// development builds are neither newer nor older than anything.
export function newer(a: string | undefined, b: string | undefined): boolean {
  const pa = parse(a);
  const pb = parse(b);
  if (!pa || !pb) return false;
  for (let i = 0; i < 3; i++) {
    if (pa[i] !== pb[i]) return pa[i] > pb[i];
  }
  return false;
}

function parse(v: string | undefined): number[] | null {
  const m = /^v?(\d+)\.(\d+)\.(\d+)$/.exec(v ?? "");
  return m ? m.slice(1).map(Number) : null;
}
