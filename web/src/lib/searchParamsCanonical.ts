/**
 * Canonical query comparison for Next.js App Router URL sync.
 * Order of keys and of repeated values per key is ignored; encoding is normalized via decode → encodeURIComponent.
 * First page: `page` absent and `page=1` (1-based) are treated as equivalent so URL sync does not oscillate.
 */

function isFirstPageParamValue(value: string): boolean {
  const v = value.trim();
  if (v === "" || v === "1") return true;
  const n = parseInt(v, 10);
  return Number.isFinite(n) && n <= 1;
}

export function canonicalSearchParamsString(input: string | { toString(): string }): string {
  const raw = typeof input === "string" ? input : input.toString();
  const trimmed = raw.replace(/^\?/, "").trim();
  if (!trimmed) return "";

  const sp = new URLSearchParams(trimmed);
  const byKey = new Map<string, string[]>();
  for (const [k, v] of sp.entries()) {
    let arr = byKey.get(k);
    if (!arr) {
      arr = [];
      byKey.set(k, arr);
    }
    arr.push(v);
  }

  const pageVals = byKey.get("page");
  if (pageVals && pageVals.length > 0 && pageVals.every(isFirstPageParamValue)) {
    byKey.delete("page");
  }

  for (const arr of byKey.values()) {
    arr.sort();
  }
  const sortedKeys = [...byKey.keys()].sort((a, b) => a.localeCompare(b));
  const parts: string[] = [];
  for (const k of sortedKeys) {
    for (const v of byKey.get(k) ?? []) {
      parts.push(`${encodeURIComponent(k)}=${encodeURIComponent(v)}`);
    }
  }
  return parts.join("&");
}

export function areSearchParamsEquivalent(
  a: string | { toString(): string },
  b: string | { toString(): string },
): boolean {
  return canonicalSearchParamsString(a) === canonicalSearchParamsString(b);
}
