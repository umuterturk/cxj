package acceptance

import (
	"net/url"
	"strings"
	"testing"
)

// Different concepts, unfamiliar identifiers and two languages. A fixed dictionary
// or source-specific query override is forbidden by AGENTS.md. This calls REAL Ollama.
func TestP3SemanticSearchFindsUnseenConceptsAcrossLanguages(t *testing.T) {
	root := t.TempDir()
	examples := []struct{ file, source, query, language string }{
		{"LoanAdmissionRule.java", `class LoanAdmissionRule {
    // Evaluate risk appetite and whether an applicant qualifies for borrowing.
    boolean canProceed() { return true; }
}`, "müşterinin kredi almaya uygunluğunu hangi metot belirliyor", "tr"},
		{"CardChargeReversal.java", `class CardChargeReversal {
    // Reverse an already captured charge when a purchase is cancelled.
    void undoCapturedFunds() {}
}`, "iptal edilen alışverişin para iadesi", "tr"},
		{"OverdueInvoiceNotifier.java", `class OverdueInvoiceNotifier {
    // Alert clients when their bills have passed the due date.
    void warnLatePayers() {}
}`, "gecikmiş fatura müşterisine uyarı gönderme", "tr"},
		{"PromotionPriceEngine.java", `class PromotionPriceEngine {
    // Apply a discounted price during a promotional sales period.
    int applyReduction(int price) { return price - 5; }
}`, "ürün fiyatı için indirim hesaplama", "tr"},
		{"UploadIntegrityVerifier.java", `class UploadIntegrityVerifier {
    // Compare document digest with expected checksum before storage.
    boolean verifyPayload(byte[] bytes) { return bytes.length > 0; }
}`, "check whether an uploaded file was corrupted in transit", "en"},
		{"WarehouseSlotAllocator.java", `class WarehouseSlotAllocator {
    // Assign available inventory storage capacity to incoming parcels.
    void reserveBay() {}
}`, "choose where arriving packages are stored", "en"},
		{"RepeatedLoginDefense.java", `class RepeatedLoginDefense {
    // Disable sign-in after successive failed password attempts.
    void denyRepeatedAttempts() {}
}`, "stop someone guessing passwords again and again", "en"},
		{"DeadLetterForwarder.java", `class DeadLetterForwarder {
    // Send unsuccessful event processing results to a separate failure queue.
    void forwardUnprocessableEvent() {}
}`, "where are events routed when consumers cannot process them", "en"},
	}
	for _, ex := range examples {
		put(t, root, "src/"+ex.file, ex.source)
	}
	e := start(t, root, true)
	e.waitSemantic(t)
	matched, totalTR, matchedTR := 0, 0, 0
	for _, ex := range examples {
		results := e.search(t, ex.query, "semantic")
		if ex.language == "tr" {
			totalTR++
		}
		if hasPath(results, ex.file) {
			matched++
			if ex.language == "tr" {
				matchedTR++
			}
		}
		t.Logf("%s / %s: hit=%v", ex.language, ex.file, hasPath(results, ex.file))
	}
	if matched < 6 {
		t.Fatalf("semantic recall@10 %d/%d; need at least 6 hits across diverse unseen concepts", matched, len(examples))
	}
	if matchedTR < 2 {
		t.Fatalf("multilingual recall %d/%d; need at least two successful Turkish->English matches", matchedTR, totalTR)
	}
}
func TestP3HybridAlwaysPreservesExactIdentifierRanking(t *testing.T) {
	root := t.TempDir()
	put(t, root, "CaptureReconciler.java", `class CaptureReconciler { void settleBatch() {} }`)
	put(t, root, "OtherReconciler.java", `class OtherReconciler { void batchSettlement() {} }`)
	e := start(t, root, true)
	e.waitSemantic(t)
	results := e.search(t, "CaptureReconciler.settleBatch", "hybrid")
	if len(results) == 0 || !strings.Contains(results[0].Symbol, "CaptureReconciler") {
		t.Fatalf("exact identifier must rank above semantically similar results: %#v", results)
	}
	if results[0].Match != "exact" {
		t.Fatalf("exact hit cannot be labeled semantic: %#v", results[0])
	}
}
func TestP3AbsentModelMustNotPretendSemanticSearchWorked(t *testing.T) {
	// Run a real engine, but with a deliberately unreachable localhost embed server.
	root := t.TempDir()
	put(t, root, "Query.java", "class Query { void execute() {} }")
	e := start(t, root, false)
	// Ensure the default server is lexical only: asking for semantic is a 503, not an empty 200.
	p := "/v1/search?q=" + url.QueryEscape("where is the query executed") + "&mode=semantic"
	status, err := e.rawStatus(p)
	if err != nil {
		t.Fatal(err)
	}
	if status != 503 {
		t.Fatalf("semantic not enabled: expected 503, got HTTP %d", status)
	}
}

func TestP3ConfiguredButUnreachableEmbeddingProviderIsAnError(t *testing.T) {
	// Prove that an unavailable configured model is reported, not silently replaced.
	t.Setenv("CX_EMBED_URL", "http://127.0.0.1:1")
	root := t.TempDir()
	put(t, root, "Policy.java", "class Policy { void examine() {} }")
	e := start(t, root, true)
	p := "/v1/search?q=" + url.QueryEscape("where to examine requests") + "&mode=hybrid"
	code, err := e.rawStatus(p)
	if err != nil {
		t.Fatal(err)
	}
	if code != 503 {
		t.Fatalf("model unavailable: hybrid query must return 503, got %d", code)
	}
}

func TestP3ExcludedJavaNeverAppearsInSemanticSearch(t *testing.T) {
	root := t.TempDir()
	put(t, root, "src/AuthorizationPolicy.java", `class AuthorizationPolicy {
       // Determine which user permissions permit a sensitive operation.
       boolean mayProceed() { return true; }
    }`)
	put(t, root, "generated/SensitiveProfile.java", `class SensitiveProfile {
       // Explain authentication credentials and user permissions.
       void revealCredentials() {}
    }`)
	e := start(t, root, true, "generated/**")
	e.waitSemantic(t)
	requireNoPath(t, e.search(t, "which permission allows someone to perform a protected action", "semantic"), "SensitiveProfile.java")
}
