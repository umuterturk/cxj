package index

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Temporary generator: emits the pinned expectation table for
// TestSearchPinnedRanking from the golden fixture, ranked by the legacy
// matcher (the pre-P7 behaviour). Output is pasted into the test as literal
// source and reviewed by hand; keep this around for when the fixture grows,
// but never regenerate blindly -- the point of the table is human review.
func TestGeneratePinnedRanking(t *testing.T) {
	if os.Getenv("CXJ_GEN_PIN") == "" {
		t.Skip("set CXJ_GEN_PIN=1 to regenerate the pinned table")
	}
	idx := buildGoldenIndex(t)

	queries := []string{
		"pay", "handle", "handler", "handleOrder",
		"OrderHandler", "ord", "Order", "OH", "oh", "v2", "V2",
		"OrderHandlerV2", "repository", "Repository", "controller", "Controller",
		"paymentapi", "com.example.paymentapi", "PaymentController.pay",
		"OrderHandler.handle", "Tiny", "missing", "ay", "getById", "IDLE", "e",
	}

	var b strings.Builder
	total := 0
	for _, q := range queries {
		hits := searchWithLegacy(idx, q, 20)
		if len(hits) == 0 {
			b.WriteString("\t\t{\"" + q + "\", nil},\n")
			continue
		}
		b.WriteString("\t\t{\"" + q + "\", []pin{\n")
		for _, h := range hits {
			total++
			b.WriteString("\t\t\t{\"" + h.Symbol + "\", ir.Match" + title(string(h.Match)) +
				", " + strconv.FormatFloat(h.Score, 'g', -1, 64) + "},\n")
		}
		b.WriteString("\t\t}},\n")
	}
	if err := os.WriteFile("/tmp/cxj-pin.txt", []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d pinned expectations across %d queries", total, len(queries))

	// Sanity: which kinds and how many definitions does the fixture hold at all.
	kinds := map[string]int{}
	for _, d := range idx.defs {
		kinds[string(d.Kind)]++
	}
	var ks []string
	for k := range kinds {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	t.Logf("fixture kinds: %v (total %d defs)", ks, len(idx.defs))
}

func title(m string) string {
	if m == "" {
		return ""
	}
	return strings.ToUpper(m[:1]) + m[1:]
}
