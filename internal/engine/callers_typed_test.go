package engine

import (
	"testing"

	"github.com/umuterturk/cxj/internal/scan"
)

// End-to-end shape of the A3 exit: `callers <endpoint>` on a REST method
// nobody calls from code answers "nobody" (plus honest unknown-receiver
// candidates), while the service that IS called answers precisely. Before
// typed receivers, both queries answered with every call named `pay`.

const callersTypedFiles = `package com.x;

@RestController
class PaymentController {
    private final PayHandler handler;
    PaymentController(PayHandler handler) { this.handler = handler; }

    @PostMapping("/v1/pay")
    String pay(String body) { return handler.handle(body); }
}

class PayHandler {
    String handle(String body) { return "ok"; }
}

class Unrelated {
    String pay(String s) { return s; }
}

class CallerOfOtherPay {
    private final Unrelated unrelated;
    CallerOfOtherPay(Unrelated unrelated) { this.unrelated = unrelated; }
    String go() { return unrelated.pay("x"); }   // NOT the controller's pay
}
`

func TestCallersTypedEndToEnd(t *testing.T) {
	root := writeTree(t, map[string]string{
		"src/main/java/com/x/Rest.java": callersTypedFiles,
	})
	eng, err := Open(root, Options{Caps: CapsFor("callers"), Scope: scan.ScopeMain})
	if err != nil {
		t.Fatal(err)
	}

	// The endpoint: no typed caller exists, and no other class's pay is
	// allowed to impersonate one.
	ctrl := eng.Callers("com.x.PaymentController.pay")
	if len(ctrl.Results) != 0 {
		t.Errorf("callers(endpoint) = %d results, want 0:\n%v", len(ctrl.Results), ctrl.Results)
	}

	// The handler actually called by the controller: found, exactly, once.
	h := eng.Callers("com.x.PayHandler.handle")
	if len(h.Results) != 1 || h.Results[0].Path != "src/main/java/com/x/Rest.java" {
		t.Fatalf("callers(handler.handle) = %+v, want the controller call site", h.Results)
	}

	// And the same-named method on the unrelated class reports exactly its
	// own typed call site — not the controller's line, not vice versa.
	u := eng.Callers("com.x.Unrelated.pay")
	if len(u.Results) != 1 {
		t.Fatalf("callers(Unrelated.pay) = %+v, want one typed caller", u.Results)
	}
}
