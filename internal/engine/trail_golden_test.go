package engine

import (
	"strings"
	"testing"

	"github.com/umuterturk/cxj/internal/ir"
	"github.com/umuterturk/cxj/internal/scan"
)

// Engine-level goldens for `trail` (Track A, A0).
//
// This fixture is collision-shaped on purpose: it reproduces the exact shape that
// made `trail` wrong on payment-api — a package named `paymentapi` (so a query for
// `pay` is a substring of the *package*), several routes, and a method name shared
// by two types. A fixture with unique names cannot detect substring fallbacks.

const (
	trailPkg     = "com.example.paymentapi"
	trailPayCtrl = trailPkg + ".PaymentController"
	trailHandler = trailPkg + ".PayHandler"
)

func trailFiles() map[string]string {
	return map[string]string{
		"src/main/java/com/example/paymentapi/PaymentController.java": `package com.example.paymentapi;

import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class PaymentController {
    private final PayHandler payHandler;

    public PaymentController(PayHandler payHandler) {
        this.payHandler = payHandler;
    }

    @PostMapping("/v1/payments")
    public String pay(String body) {
        return payHandler.handle(body);
    }

    @GetMapping("/v1/payments/health")
    public String health() {
        return "ok";
    }
}
`,
		"src/main/java/com/example/paymentapi/BackofficePaymentController.java": `package com.example.paymentapi;

import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class BackofficePaymentController {
    @GetMapping("/v1/backoffice/payments")
    public String getPayments() {
        return "";
    }
}
`,
		"src/main/java/com/example/paymentapi/PayHandler.java": `package com.example.paymentapi;

import org.springframework.stereotype.Service;

@Service
public class PayHandler {
    private final PayRepository payRepository;

    public PayHandler(PayRepository payRepository) {
        this.payRepository = payRepository;
    }

    public String handle(String body) {
        payRepository.save(body);
        return body;
    }
}
`,
		// Same simple method name, different owner: the A-RC1 collision.
		"src/main/java/com/example/paymentapi/RefundHandler.java": `package com.example.paymentapi;

import org.springframework.stereotype.Service;

@Service
public class RefundHandler {
    public String handle(String body) {
        return refund(body);
    }

    private String refund(String body) {
        return body;
    }
}
`,
		"src/main/java/com/example/paymentapi/PayRepository.java": `package com.example.paymentapi;

import org.springframework.stereotype.Repository;

@Repository
public class PayRepository {
    public void save(String body) { }
}
`,
		// An UNMAPPED method named pay on a different type — the shape that made
		// `trail …CardPaymentClient.pay` on payment-api list POST /v1/payments: a
		// suffix match on the simple name `pay` pulled in another class's route.
		// No route here, so the contract is exactly zero http rows.
		"src/main/java/com/example/paymentapi/CardPaymentClient.java": `package com.example.paymentapi;

import org.springframework.stereotype.Component;

@Component
public class CardPaymentClient {
    public String pay(String body) {
        return body;
    }
}
`,
		"src/test/java/com/example/paymentapi/PaymentControllerTest.java": `package com.example.paymentapi;

public class PaymentControllerTest {
    public void pays() { }
}
`,
		// Shares the "pay" prefix but is not a test of PaymentController.
		"src/test/java/com/example/paymentapi/PayoutTest.java": `package com.example.paymentapi;

public class PayoutTest {
    public void checks() { }
}
`,
		// Qualified test names are the common convention in real Spring repos
		// (payment-api has PaymentControllerValidationTest, not PaymentControllerTest).
		// An over-strict matcher reports the class as untested, which is its own bug.
		"src/test/java/com/example/paymentapi/PaymentControllerValidationTest.java": `package com.example.paymentapi;

public class PaymentControllerValidationTest {
    public void rejects() { }
}
`,
	}
}

func openTrailEngine(t *testing.T) *Engine {
	t.Helper()
	eng, err := Open(writeTree(t, trailFiles()), Options{Caps: CapsFor("trail"), Scope: scan.ScopeMain})
	if err != nil {
		t.Fatal(err)
	}
	return eng
}

func hitSymbols(hits []ir.TrailHit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Symbol)
	}
	return out
}

// TestTrailResolvesTheQueriedSymbol is the base contract: the card is about the
// symbol asked for, at its real location.
func TestTrailResolvesTheQueriedSymbol(t *testing.T) {
	eng := openTrailEngine(t)
	tr := eng.Trail(trailPayCtrl + ".pay")
	if tr.Path == "" {
		t.Fatal("trail did not resolve PaymentController.pay")
	}
	if !strings.HasSuffix(tr.Path, "PaymentController.java") {
		t.Fatalf("trail resolved to the wrong file: %s", tr.Path)
	}
	if tr.Line <= 0 {
		t.Fatalf("trail has no line: %+v", tr)
	}
}

// TestTrailCalleesStayInOwnersFile: a method query returns the calls that method
// makes. This holds today for `pay` (only PaymentController.pay has a body) and is
// the property A2 must preserve while it tightens the matcher.
func TestTrailCalleesStayInOwnersFile(t *testing.T) {
	eng := openTrailEngine(t)
	for _, h := range eng.Trail(trailPayCtrl + ".pay").Callees {
		if !strings.HasSuffix(h.Path, "PaymentController.java") {
			t.Errorf("callee outside owner's file: %s %s", h.Path, h.Symbol)
		}
	}
}

// TestTrailShowsItsOwnRoute is the http contract: a mapped method's card lists the
// route it is mapped to. Written positively (must contain) so it passes before and
// after; A1 adds the negative half (must not contain other classes' routes).
func TestTrailShowsItsOwnRoute(t *testing.T) {
	eng := openTrailEngine(t)
	hits := eng.Trail(trailPayCtrl + ".pay").HTTP
	if len(hits) == 0 {
		t.Fatal("trail shows no http route for a @PostMapping method")
	}
	var own bool
	for _, h := range hits {
		if h.Symbol == trailPayCtrl+".pay" {
			own = true
			if !strings.Contains(h.Detail, "POST") || !strings.Contains(h.Detail, "/v1/payments") {
				t.Errorf("own route detail wrong: %s", h.Detail)
			}
		}
	}
	if !own {
		t.Errorf("trail omitted the route actually declared on the method: %v", hitSymbols(hits))
	}
}

// TestTrailInjectsAreTheOwnersOwnFields locks the injections section to the
// queried type's own dependencies.
func TestTrailInjectsAreTheOwnersOwnFields(t *testing.T) {
	eng := openTrailEngine(t)
	hits := eng.Trail(trailPayCtrl).Injects
	if len(hits) == 0 {
		t.Fatal("no injections found for a type with a constructor dependency")
	}
	for _, h := range hits {
		if !strings.HasSuffix(h.Path, "PaymentController.java") {
			t.Errorf("injection outside the type: %s %s", h.Path, h.Symbol)
		}
	}
}

// --- A1: substring fallbacks -------------------------------------------------
//
// These are the negative halves of the contract above. They are the failure the
// bug report was actually about: `trail …PaymentController.pay` on payment-api
// listed five routes owned by other classes because the matcher fell back to
// Contains, and `paymentapi` contains `pay`.

// TestTrailHTTPOnlyShowsOwnRoutes: every http row on a method's card must be a
// route that method or its own class actually declares.
func TestTrailHTTPOnlyShowsOwnRoutes(t *testing.T) {
	eng := openTrailEngine(t)
	hits := eng.Trail(trailPayCtrl + ".pay").HTTP
	for _, h := range hits {
		if h.Symbol != trailPayCtrl+".pay" && !strings.HasPrefix(h.Symbol, trailPayCtrl+".") {
			t.Errorf("http row belongs to another class: %s (%s)", h.Symbol, h.Detail)
		}
	}
}

// TestTrailHTTPForATypeShowsOnlyItsOwnRoutes, same rule for a type query: a
// controller's card lists its routes, not every route in the package.
func TestTrailHTTPForATypeShowsOnlyItsOwnRoutes(t *testing.T) {
	eng := openTrailEngine(t)
	for _, h := range eng.Trail(trailPayCtrl).HTTP {
		if !strings.HasPrefix(h.Symbol, trailPayCtrl+".") {
			t.Errorf("type card shows a route it does not own: %s (%s)", h.Symbol, h.Detail)
		}
	}
}

// TestTrailHTTPEmptyForUnmappedSameNameMethod is the exit criterion measured on
// payment-api (A8): `trail …CardPaymentClient.pay` — an unmapped method whose
// simple name collides with a mapped one — must show NO routes. The suffix
// fallback made every `*.pay` method in the repo claim POST /v1/payments.
func TestTrailHTTPEmptyForUnmappedSameNameMethod(t *testing.T) {
	eng := openTrailEngine(t)
	hits := eng.Trail(trailPkg + ".CardPaymentClient.pay").HTTP
	if len(hits) != 0 {
		t.Errorf("unmapped pay claimed routes it does not declare: %v", hitSymbols(hits))
	}
}

// TestTrailTestsMatchTheOwnerType: the tests section is "what covers this class".
// Asking about the `pay` *method* must not drag in every type whose name merely
// contains "pay" — PayoutTest shares a prefix with the package, not with the class.
func TestTrailTestsMatchTheOwnerType(t *testing.T) {
	eng, err := Open(writeTree(t, trailFiles()), Options{Caps: CapsFor("trail")})
	if err != nil {
		t.Fatal(err)
	}
	hits := eng.Trail(trailPayCtrl + ".pay").Tests
	syms := strings.Join(hitSymbols(hits), " ")
	if !strings.Contains(syms, "PaymentControllerTest") {
		t.Errorf("tests section missed the class's own test: %v", hitSymbols(hits))
	}
	if !strings.Contains(syms, "PaymentControllerValidationTest") {
		t.Errorf("tests section dropped a qualified test name: %v", hitSymbols(hits))
	}
	if strings.Contains(syms, "PayoutTest") {
		t.Errorf("tests section matched PayoutTest by substring: %v", hitSymbols(hits))
	}
}

// TestContextDoesNotMatchPackageSubstring: `context pay` must answer about a
// symbol named pay, not about everything under com.example.paymentapi.
func TestContextDoesNotMatchPackageSubstring(t *testing.T) {
	eng := openTrailEngine(t)
	raw := eng.Context("pay", 50)
	ctx, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected context shape %T", raw)
	}
	locations, _ := ctx["locations"].([]map[string]any)
	for _, loc := range locations {
		p, _ := loc["path"].(string)
		if strings.Contains(p, "BackofficePaymentController") {
			t.Errorf("context matched an unrelated class through the package name: %v", loc)
		}
	}
}

// --- A2: callees must be owner-scoped ---------------------------------------

// TestCalleesOfSharedMethodIsOwnerScoped is the engine view of the 292-results bug:
// `handle` exists on PayHandler and RefundHandler here, and asking for one by
// qualified name must not return the other's call sites.
func TestCalleesOfSharedMethodIsOwnerScoped(t *testing.T) {
	eng := openTrailEngine(t)
	reply := eng.Callees(trailHandler + ".handle")
	if len(reply.Results) == 0 {
		t.Fatal("no callees for PayHandler.handle")
	}
	for _, r := range reply.Results {
		if !strings.HasSuffix(r.Path, "PayHandler.java") {
			t.Errorf("callee from another owner leaked in: %s %s", r.Path, r.Target)
		}
	}
}

// TestTrailCalleesOfSharedMethodStayInOwnerFile, same rule on the card.
func TestTrailCalleesOfSharedMethodStayInOwnerFile(t *testing.T) {
	eng := openTrailEngine(t)
	for _, h := range eng.Trail(trailHandler + ".handle").Callees {
		if !strings.HasSuffix(h.Path, "PayHandler.java") {
			t.Errorf("trail callee outside the owner's file: %s %s", h.Path, h.Symbol)
		}
	}
}

// TestCalleesReportsAmbiguity: a bare method name that several types own must say
// so. Silently answering about one arbitrary owner is how an agent ends up reading
// the wrong class and believing it is the right one.
func TestCalleesReportsAmbiguity(t *testing.T) {
	eng := openTrailEngine(t)
	reply := eng.Callees("handle")
	if len(reply.Ambiguous) < 2 {
		t.Fatalf("bare `handle` should be flagged ambiguous, got %v", reply.Ambiguous)
	}
	var sawPayHandler, sawRefundHandler bool
	for _, a := range reply.Ambiguous {
		switch {
		case strings.HasSuffix(a, "PayHandler.handle"):
			sawPayHandler = true
		case strings.HasSuffix(a, "RefundHandler.handle"):
			sawRefundHandler = true
		}
	}
	if !sawPayHandler || !sawRefundHandler {
		t.Errorf("ambiguity list incomplete: %v", reply.Ambiguous)
	}
}

// TestCalleesOfQualifiedSymbolIsNotAmbiguous: the flag must stay quiet when the
// query already names one owner, or it becomes noise and gets ignored.
func TestCalleesOfQualifiedSymbolIsNotAmbiguous(t *testing.T) {
	eng := openTrailEngine(t)
	reply := eng.Callees(trailHandler + ".handle")
	if len(reply.Ambiguous) != 0 {
		t.Errorf("qualified query flagged ambiguous: %v", reply.Ambiguous)
	}
}

// --- A6/A7 at the surface ----------------------------------------------------

// TestSearchSymbolHitOutranksTextHit is why the match-tier table had to move into
// ir: engine kept its own copy for merging symbol and text hits, so a tier the
// copy did not know about sorted below raw grep matches and a class named
// PayHandler lost to a line in demo.html.
func TestSearchSymbolHitOutranksTextHit(t *testing.T) {
	files := map[string]string{
		"src/main/java/com/example/paymentapi/PayHandler.java": `package com.example.paymentapi;

public class PayHandler {
    public String handle(String body) { return body; }
}
`,
		"src/main/resources/application.yml": "app:\n  handler: pay\n  other-handler: refund\n",
	}
	eng, err := Open(writeTree(t, files), Options{Caps: CapsFor("find")})
	if err != nil {
		t.Fatal(err)
	}
	results := eng.Search("Handler", 10).Results
	if len(results) == 0 {
		t.Fatal("nothing found")
	}
	if results[0].Kind != ir.KindClass {
		t.Fatalf("a raw text line outranked the class: %s %s", results[0].Kind, results[0].Path)
	}
	var sawText bool
	for _, r := range results {
		if r.Kind == ir.KindText {
			sawText = true
		}
	}
	if !sawText {
		t.Error("text hits disappeared; the tier is a sink, not a filter")
	}
}
