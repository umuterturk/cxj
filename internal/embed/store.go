package embed

import (
	"math"
	"sort"
	"sync"

	"example.com/cx/internal/ir"
)

// Hit is one semantic search result before IR conversion.
type Hit struct {
	Chunk Chunk
	Score float64
}

// Store holds float32 vectors entirely in RAM.
type Store struct {
	mu     sync.RWMutex
	chunks []Chunk
	vecs   [][]float32
}

// Add appends a normalized vector for chunk.
func (s *Store) Add(chunk Chunk, vec []float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chunks = append(s.chunks, chunk)
	s.vecs = append(s.vecs, vec)
}

// Len returns indexed chunk count.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.chunks)
}

// Search brute-force dot product (vectors assumed L2-normalized ⇒ cosine).
func (s *Store) Search(query []float32, limit int) []Hit {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || len(s.vecs) == 0 {
		return nil
	}
	type scored struct {
		i int
		s float64
	}
	scores := make([]scored, 0, len(s.vecs))
	for i, v := range s.vecs {
		scores = append(scores, scored{i: i, s: dot(query, v)})
	}
	sort.Slice(scores, func(a, b int) bool { return scores[a].s > scores[b].s })
	if len(scores) > limit {
		scores = scores[:limit]
	}
	out := make([]Hit, 0, len(scores))
	seen := map[string]struct{}{}
	for _, sc := range scores {
		c := s.chunks[sc.i]
		key := c.Path + "|" + c.Symbol
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, Hit{Chunk: c, Score: sc.s})
	}
	return out
}

// ToResults converts hits to search results.
func ToResults(hits []Hit, match ir.MatchKind) []ir.SearchResult {
	out := make([]ir.SearchResult, 0, len(hits))
	for _, h := range hits {
		out = append(out, ir.SearchResult{
			Path:    h.Chunk.Path,
			Line:    h.Chunk.Line,
			Symbol:  h.Chunk.Symbol,
			Kind:    h.Chunk.Kind,
			Match:   match,
			Score:   h.Score,
			Snippet: truncateRunes(h.Chunk.Text, 160),
		})
	}
	return out
}

func dot(a, b []float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var sum float64
	for i := 0; i < n; i++ {
		sum += float64(a[i]) * float64(b[i])
	}
	if math.IsNaN(sum) {
		return 0
	}
	return sum
}

// RRF merges ranked lists with reciprocal rank fusion; exact lexical hits stay first.
func RRF(exactLexical, lexical, semantic []ir.SearchResult, limit int) []ir.SearchResult {
	const k = 60.0
	type agg struct {
		res   ir.SearchResult
		score float64
		exact bool
	}
	byKey := map[string]*agg{}
	add := func(list []ir.SearchResult, weight float64, forceExact bool) {
		for i, r := range list {
			key := r.Path + "|" + r.Symbol + "|" + itoa(r.Line)
			a, ok := byKey[key]
			if !ok {
				cp := r
				a = &agg{res: cp}
				byKey[key] = a
			}
			a.score += weight / (k + float64(i+1))
			if forceExact || r.Match == ir.MatchExact {
				a.exact = true
				a.res.Match = ir.MatchExact
			} else if !a.exact && a.res.Match != ir.MatchExact {
				if r.Match == ir.MatchHybrid || r.Match == ir.MatchSemantic || a.res.Match == "" {
					a.res.Match = ir.MatchHybrid
				}
			}
			if r.Score > a.res.Score {
				a.res.Score = r.Score
			}
			if a.res.Snippet == "" {
				a.res.Snippet = r.Snippet
			}
		}
	}
	add(exactLexical, 2.0, true)
	add(lexical, 1.0, false)
	add(semantic, 1.0, false)

	all := make([]*agg, 0, len(byKey))
	for _, a := range byKey {
		all = append(all, a)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].exact != all[j].exact {
			return all[i].exact
		}
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		if all[i].res.Path != all[j].res.Path {
			return all[i].res.Path < all[j].res.Path
		}
		return all[i].res.Line < all[j].res.Line
	})
	if len(all) > limit {
		all = all[:limit]
	}
	out := make([]ir.SearchResult, 0, len(all))
	for _, a := range all {
		r := a.res
		if a.exact {
			r.Match = ir.MatchExact
		} else if r.Match != ir.MatchExact {
			r.Match = ir.MatchHybrid
		}
		r.Score = a.score
		out = append(out, r)
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
