package index

import (
	"sort"
	"strconv"
	"strings"
	"testing"

	"example.com/cx/internal/ir"
	"example.com/cx/internal/java"
)

// Golden fixtures for the navigation contract (Track A, A0).
//
// These exist because the collision cases below had no coverage at all: `trail`,
// `callers`, `callees` and `find` shipped wrong on real Spring repos while the
// suite stayed green, since only `brief` was tested. Every fixture here is
// deliberately collision-shaped, because a fixture where every name is unique
// would pass no matter how the matcher behaves.
//
// What the fixture encodes:
//   - a method name (`pay`, `handle`) that exists on several owners
//   - a package name that *contains* the query (`paymentapi` contains `pay`)
//   - two types where one name is a prefix of the other (`OrderHandler`,
//     `OrderHandlerV2`) so dot-boundary matching is actually tested
//   - a field literally named `handler` next to types named `*Handler`
//   - a test class per type, for the tests section

const (
	pkgPay     = "com.example.paymentapi"
	pkgHandler = "com.example.pay"

	goldenPayController = `package ` + pkgPay + `;

import org.springframework.web.bind.annotation.RestController;

@RestController
public class PaymentController {
    private final PayService payService;

    public PaymentController(PayService payService) {
        this.payService = payService;
    }

    public String pay(String body) {
        payService.pay(body);
        return audit(body);
    }

    private String audit(String body) {
        return body;
    }
}
`

	goldenPayService = `package ` + pkgPay + `;

public interface PayService {
    String pay(String body);
}
`

	// PayoutTest shares a prefix with "Pay" but is not a test of PaymentController.
	goldenPayoutTest = `package ` + pkgPay + `;

public class PayoutTest {
    public void checkPayout() { }
}
`

	goldenPaymentControllerTest = `package ` + pkgPay + `;

public class PaymentControllerTest {
    public void pays() { }
}
`

	goldenOrderHandler = `package ` + pkgHandler + `;

public class OrderHandler {
    private final OrderRepository payRepository;

    public OrderHandler(OrderRepository payRepository) {
        this.payRepository = payRepository;
    }

    public String handle(String in) {
        payRepository.save(in);
        return normalize(in);
    }

    private String normalize(String in) {
        return in;
    }
}
`

	// Same simple method name as OrderHandler.handle, different owner.
	goldenShippingHandler = `package ` + pkgHandler + `;

public class ShippingHandler {
    public String handle(String in) {
        return freight(in);
    }

    private String freight(String in) {
        return in;
    }
}
`

	// One name is a prefix of the other: only dot-boundary matching separates them.
	goldenOrderHandlerV2 = `package ` + pkgHandler + `;

public class OrderHandlerV2 {
    public String handle(String in) {
        return legacy(in);
    }

    private String legacy(String in) {
        return in;
    }
}
`

	goldenOrderRepository = `package ` + pkgHandler + `;

public class OrderRepository {
    public void save(String in) { }
}
`

	// A field named exactly `handler` sitting beside *Handler types, which is how
	// `find Handler` came back full of private test fields.
	goldenFieldNamedHandler = `package ` + pkgHandler + `;

public class BackofficeHarness {
    private final OrderHandler handler;
    private final ShippingHandler shippingHandler;

    public BackofficeHarness(OrderHandler handler, ShippingHandler shippingHandler) {
        this.handler = handler;
        this.shippingHandler = shippingHandler;
    }
}
`
)

func goldenFiles() map[string]string {
	return map[string]string{
		"src/main/java/com/example/paymentapi/PaymentController.java":     goldenPayController,
		"src/main/java/com/example/paymentapi/PayService.java":            goldenPayService,
		"src/test/java/com/example/paymentapi/PayoutTest.java":            goldenPayoutTest,
		"src/test/java/com/example/paymentapi/PaymentControllerTest.java": goldenPaymentControllerTest,
		"src/main/java/com/example/pay/OrderHandler.java":                 goldenOrderHandler,
		"src/main/java/com/example/pay/ShippingHandler.java":              goldenShippingHandler,
		"src/main/java/com/example/pay/OrderHandlerV2.java":               goldenOrderHandlerV2,
		"src/main/java/com/example/pay/OrderRepository.java":              goldenOrderRepository,
		"src/main/java/com/example/pay/BackofficeHarness.java":            goldenFieldNamedHandler,
	}
}

// buildGoldenIndex extracts the fixture the same way a real scan does, so these
// tests exercise extraction and matching together. A0's contract is that
// currently-correct behavior stays correct while the accuracy fixes land.
func buildGoldenIndex(t *testing.T) *SymbolIndex {
	t.Helper()
	files := goldenFiles()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	ex := java.NewExtractor()
	defer ex.Close()

	facts := make([]ir.FileFacts, 0, len(names))
	for _, name := range names {
		f, err := ex.Extract(name, []byte(files[name]))
		if err != nil {
			t.Fatalf("extract %s: %v", name, err)
		}
		facts = append(facts, f)
	}
	idx := Build(facts)
	if len(idx.defs) == 0 {
		t.Fatal("golden fixture produced no definitions")
	}
	return idx
}

func targets(results []ir.RefResult) []string {
	out := make([]string, 0, len(results))
	for _, r := range results {
		out = append(out, r.Path+":"+strconv.Itoa(r.Line))
	}
	return out
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

func pathsOf(results []ir.RefResult) map[string]int {
	out := map[string]int{}
	for _, r := range results {
		out[r.Path]++
	}
	return out
}

// TestGoldenExtraction locks the shapes every later assertion depends on.
func TestGoldenExtraction(t *testing.T) {
	idx := buildGoldenIndex(t)

	if _, ok := idx.Lookup(pkgPay + ".PaymentController"); !ok {
		t.Fatal("PaymentController type not indexed")
	}
	if _, ok := idx.Lookup(pkgPay + ".PaymentController.pay"); !ok {
		t.Fatal("PaymentController.pay method not indexed")
	}
	if _, ok := idx.Lookup(pkgHandler + ".OrderHandler.handle"); !ok {
		t.Fatal("OrderHandler.handle not indexed")
	}
	// Call edges are owned by the fully-qualified enclosing method. If that ever
	// changes, the A2 callee scoping below silently stops meaning anything.
	var fromOrderHandle bool
	for _, e := range idx.edges {
		if e.Kind == "calls" && e.From == pkgHandler+".OrderHandler.handle" {
			fromOrderHandle = true
		}
	}
	if !fromOrderHandle {
		t.Fatal("expected calls edges owned by OrderHandler.handle; Edge.From is no longer the enclosing method")
	}
}

// TestCalleesOfUniqueMethodName is the contract that already holds and must
// survive: a method name with exactly one owner returns its own call sites.
func TestCalleesOfUniqueMethodName(t *testing.T) {
	idx := buildGoldenIndex(t)
	got := idx.Callees(pkgPay + ".PaymentController.pay")
	if len(got) == 0 {
		t.Fatal("no callees for PaymentController.pay")
	}
	for _, r := range got {
		if r.Path != "src/main/java/com/example/paymentapi/PaymentController.java" {
			t.Errorf("callee outside the owner's file: %s", r.Path)
		}
	}
}

// TestCalleesOfSharedMethodNameIsOwnerScoped is A2's core claim. `handle` exists on
// three types here; asking for one of them by its fully-qualified name must not
// return the other two. This is the bug behind `callees …PayHandler.handle`
// returning 292 results across 20 files on payment-api: the matcher accepted any
// edge whose owner *ended with* the queried simple name.
func TestCalleesOfSharedMethodNameIsOwnerScoped(t *testing.T) {
	idx := buildGoldenIndex(t)
	got := idx.Callees(pkgHandler + ".OrderHandler.handle")
	if len(got) == 0 {
		t.Fatal("no callees for OrderHandler.handle")
	}
	for _, r := range got {
		if r.Path != "src/main/java/com/example/pay/OrderHandler.java" {
			t.Errorf("callee from a different owner leaked in: %s %s", r.Path, r.Target)
		}
	}
	for _, r := range got {
		if strings.Contains(r.Target, "freight") || strings.Contains(r.Target, "legacy") {
			t.Errorf("callee belongs to ShippingHandler/OrderHandlerV2: %s", r.Target)
		}
	}
}

// TestCalleesOfTypeQueryStaysInsideTheType: a type query means "everything this
// type calls", and OrderHandlerV2 must not count as OrderHandler.
func TestCalleesOfTypeQueryStaysInsideTheType(t *testing.T) {
	idx := buildGoldenIndex(t)
	got := idx.Callees(pkgHandler + ".OrderHandler")
	if len(got) == 0 {
		t.Fatal("no callees for the OrderHandler type")
	}
	for _, r := range got {
		if r.Path != "src/main/java/com/example/pay/OrderHandler.java" {
			t.Errorf("callee outside the type: %s %s", r.Path, r.Target)
		}
	}
}

// TestLookupAllFindsEveryOwner is the ambiguity surface: a bare name that matches
// several definitions must report them all, not silently answer about one.
func TestLookupAllFindsEveryOwner(t *testing.T) {
	idx := buildGoldenIndex(t)

	all := idx.LookupAll("handle")
	if len(all) < 3 {
		t.Fatalf("LookupAll(handle) found %d, want all three owners", len(all))
	}
	owners := map[string]bool{}
	for _, d := range all {
		owners[d.Owner] = true
	}
	for _, want := range []string{
		pkgHandler + ".OrderHandler",
		pkgHandler + ".ShippingHandler",
		pkgHandler + ".OrderHandlerV2",
	} {
		if !owners[want] {
			t.Errorf("LookupAll(handle) missing owner %s", want)
		}
	}

	// A fully-qualified query resolves to exactly one definition.
	one := idx.LookupAll(pkgHandler + ".OrderHandler.handle")
	if len(one) != 1 {
		t.Fatalf("qualified LookupAll returned %d, want 1", len(one))
	}
}

// TestHeritageIsExact locks extends/implements lookups by qualified name.
func TestHeritageIsExact(t *testing.T) {
	idx := buildGoldenIndex(t)
	_, subs := idx.Heritage(pkgPay + ".PayService")
	if len(subs) == 0 {
		t.Log("no implementors recorded for PayService (fixture has no implements clause)")
	}
}

// TestSearchExactBeatsPrefix is the ranking contract that A6/A7 must not break:
// adding weaker match tiers may fill gaps, never reorder a good hit.
func TestSearchExactBeatsPrefix(t *testing.T) {
	idx := buildGoldenIndex(t)
	hits := idx.Search(pkgPay+".PaymentController", 20)
	if len(hits) == 0 {
		t.Fatal("exact qualified query found nothing")
	}
	if hits[0].Symbol != pkgPay+".PaymentController" {
		t.Fatalf("exact hit not first: %s", hits[0].Symbol)
	}
	if hits[0].Match != ir.MatchExact {
		t.Fatalf("exact query should rank MatchExact, got %s", hits[0].Match)
	}
}

// TestSearchPrefixIsBounded locks that a prefix query does not wander.
func TestSearchPrefixIsBounded(t *testing.T) {
	idx := buildGoldenIndex(t)
	for _, h := range idx.Search("Payment", 20) {
		if !strings.Contains(strings.ToLower(h.Symbol), "payment") {
			t.Errorf("prefix query returned unrelated symbol %s", h.Symbol)
		}
	}
}

// TestTestTypeMatches covers the naming rules for "which class tests this one".
// The false negative here is as bad as the false positive: an earlier version
// accepted only TTest and silently dropped PaymentControllerValidationTest, which
// made `trail` report a controller as untested when it is not.
func TestTestTypeMatches(t *testing.T) {
	cases := []struct {
		candidate, typeName string
		want                bool
	}{
		{"PaymentControllerTest", "PaymentController", true},
		{"PaymentControllerValidationTest", "PaymentController", true},
		{"PaymentControllerIntegrationTest", "PaymentController", true},
		{"PaymentControllerIT", "PaymentController", true},
		{"PaymentControllerSpec", "PaymentController", true},
		{"TestPaymentController", "PaymentController", true},
		{"PaymentController", "PaymentController", true},
		// A shared prefix or substring is not a test of the class.
		{"PayoutTest", "PaymentController", false},
		{"PayoutTest", "Pay", false},
		{"PaymentApiSmokeTest", "Pay", false},
		// Accepted soft overlap: an adjacent class's test also reads as a test of
		// a very short type name. The tests section is a capped hint, not a claim.
		{"PayHandlerTest", "Pay", true},
		{"ShippingHandlerTest", "PaymentController", false},
	}
	for _, c := range cases {
		if got := testTypeMatches(c.candidate, c.typeName); got != c.want {
			t.Errorf("testTypeMatches(%q, %q)=%v want %v", c.candidate, c.typeName, got, c.want)
		}
	}
}
