package engine

import (
	"strings"
	"testing"

	"example.com/cx/internal/ir"
	"example.com/cx/internal/scan"
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
	}
}

func openTrailEngine(t *testing.T) *Engine {
	t.Helper()
	eng, err := Open(writeTree(t, trailFiles()), Options{Scope: scan.ScopeMain})
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
