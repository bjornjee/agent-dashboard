// Subsequence fuzzy matcher used by the search overlay.
// Pure, dependency-free, deterministic. O(N*H) per call where N is the
// needle length and H is the haystack length — fine for the short
// strings (repo names, branch names) the overlay searches over.

const WORD_BOUNDARY = /[-_/. ]/;

// fuzzyMatch returns { score, indices } when every needle char appears
// in order inside haystack (case-insensitive), or null otherwise.
// Empty needle → { score: 0, indices: [] }.
export function fuzzyMatch(needle, haystack) {
  if (typeof haystack !== 'string') return null;
  if (!needle) return { score: 0, indices: [] };

  const n = needle.toLowerCase();
  const h = haystack.toLowerCase();
  const N = n.length;
  const H = h.length;
  if (N > H) return null;

  // cells[i][j] is the best alignment of needle[0..i] within haystack[0..j],
  // or null if no alignment exists. `lastIdx` is the haystack index used
  // for the i-th needle char (so consecutive bonuses can be detected).
  const cells = Array.from({ length: N + 1 }, () => new Array(H + 1).fill(null));
  const base = { score: 0, indices: [], lastIdx: -1 };
  for (let j = 0; j <= H; j++) cells[0][j] = base;

  for (let i = 1; i <= N; i++) {
    for (let j = 1; j <= H; j++) {
      // Option A: skip haystack[j-1].
      let best = cells[i][j - 1];
      // Option B: consume haystack[j-1] as the i-th needle char.
      if (n[i - 1] === h[j - 1]) {
        const prev = cells[i - 1][j - 1];
        if (prev != null) {
          let add = 1;
          if (prev.lastIdx === j - 2 && i > 1) add += 3; // consecutive run
          if (i === 1) {
            if (j - 1 === 0) add += 4; // prefix bonus
            else if (WORD_BOUNDARY.test(h[j - 2])) add += 2; // word boundary
          }
          const candidate = {
            score: prev.score + add,
            indices: prev.indices.concat(j - 1),
            lastIdx: j - 1,
          };
          if (best == null || candidate.score > best.score) best = candidate;
        }
      }
      cells[i][j] = best;
    }
  }

  const final = cells[N][H];
  if (final == null) return null;
  return { score: final.score, indices: final.indices };
}

// fuzzyRank applies fuzzyMatch to multiple fields per item and returns
// the matching items sorted by best per-item score (descending), with
// original index as a stable tie-breaker.
//
// `getHaystacks(item)` must return an array of strings; the returned
// `indicesByField` is parallel to that array, with `null` for fields
// that did not match.
//
// Empty needle returns every item with score 0 in original order
// (and `indicesByField` populated with empty arrays).
export function fuzzyRank(needle, items, getHaystacks) {
  const results = [];

  if (!needle) {
    for (let i = 0; i < items.length; i++) {
      const hs = getHaystacks(items[i]);
      results.push({
        item: items[i],
        score: 0,
        indicesByField: hs.map(() => []),
        _order: i,
      });
    }
    return results.map(stripOrder);
  }

  for (let i = 0; i < items.length; i++) {
    const hs = getHaystacks(items[i]);
    let bestScore = -Infinity;
    let anyMatch = false;
    const indicesByField = hs.map((s) => {
      const r = fuzzyMatch(needle, s);
      if (r == null) return null;
      anyMatch = true;
      if (r.score > bestScore) bestScore = r.score;
      return r.indices;
    });
    if (!anyMatch) continue;
    results.push({
      item: items[i],
      score: bestScore,
      indicesByField,
      _order: i,
    });
  }

  results.sort((a, b) => (b.score - a.score) || (a._order - b._order));
  return results.map(stripOrder);
}

function stripOrder(r) {
  return { item: r.item, score: r.score, indicesByField: r.indicesByField };
}
