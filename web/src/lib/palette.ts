// Command palette matching: a fuzzy subsequence score, and ranking within fixed groups.

export type Match = { score: number; hits: number[] }

const boundary = (c: string | undefined) => c === undefined || /[\s./_\-@:]/.test(c)

/**
 * Fuzzy-match `q` against `text` (case-insensitive): every query character must appear in order.
 * Prefixes, word starts and runs score higher; null when it does not match.
 */
export function fuzzy(q: string, text: string): Match | null {
  const query = q.trim().toLowerCase()
  if (!query) return { score: 0, hits: [] }
  const t = text.toLowerCase()
  // A contiguous match wins outright; the earlier and the more word-aligned, the better.
  const at = t.indexOf(query)
  if (at >= 0) {
    const hits = Array.from(query, (_, k) => at + k)
    return { score: 100 + (at === 0 ? 50 : boundary(t[at - 1]) ? 25 : 0) - at * 0.5 - (t.length - query.length) * 0.1, hits }
  }
  const hits: number[] = []
  let score = 0
  let from = 0
  for (const ch of query) {
    if (ch === ' ') continue
    // Prefer the next occurrence at a word start, else the next one at all.
    let i = -1
    for (let k = from; k < t.length; k++) {
      if (t[k] !== ch) continue
      if (i < 0) i = k
      if (boundary(t[k - 1])) {
        i = k
        break
      }
    }
    if (i < 0) return null
    score += boundary(t[i - 1]) ? 8 : hits.length && hits.at(-1) === i - 1 ? 5 : 1
    score -= Math.min(i - from, 6) * 0.3
    hits.push(i)
    from = i + 1
  }
  return { score, hits }
}

/** Split `text` into plain and matched runs for highlighting. */
export function highlight(text: string, hits: number[]): { text: string; hit: boolean }[] {
  const set = new Set(hits)
  const out: { text: string; hit: boolean }[] = []
  for (let i = 0; i < text.length; i++) {
    const hit = set.has(i)
    if (out.length && out.at(-1)!.hit === hit) out.at(-1)!.text += text[i]
    else out.push({ text: text[i], hit })
  }
  return out
}

const wordStart = (text: string, q: string) => {
  const t = text.toLowerCase()
  const i = t.indexOf(q.toLowerCase())
  return i === 0 || (i > 0 && boundary(t[i - 1]))
}

export type Candidate = { id: string; group: string; label: string; keywords?: string }
export type Ranked<T extends Candidate> = T & { match: Match }

/**
 * The matching items, best first within each group, groups in `order`, at most `limit[group]` each.
 * Keywords (descriptions, synonyms) match only as a whole word start, below the label, without highlights:
 * loose subsequences of long descriptions would match almost anything.
 */
export function rank<T extends Candidate>(items: T[], q: string, order: string[], limit: Record<string, number> = {}): Ranked<T>[] {
  const out: Ranked<T>[] = []
  for (const group of order) {
    const scored: Ranked<T>[] = []
    for (const it of items) {
      if (it.group !== group) continue
      const m = fuzzy(q, it.label)
      if (m) scored.push({ ...it, match: m })
      else if (q.trim() && it.keywords && wordStart(it.keywords, q.trim())) scored.push({ ...it, match: { score: 40, hits: [] } })
    }
    scored.sort((a, b) => b.match.score - a.match.score || a.label.length - b.label.length || a.label.localeCompare(b.label))
    out.push(...scored.slice(0, limit[group] ?? 50))
  }
  return out
}
