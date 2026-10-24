package acceptance

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestP4SpringControllerCombinesClassAndMethodRoutes(t *testing.T) {
	root := t.TempDir()
	put(t, root, "src/InvoiceController.java", `import org.springframework.web.bind.annotation.*;
@RestController
@RequestMapping("/invoices")
class InvoiceController {
    @GetMapping("/{id}")
    String read() { return "ok"; }
    @PostMapping
    String create() { return "ok"; }
}
`)
	e := start(t, root, false)
	var get endpointReply
	if err := e.get("/v1/endpoints?method=GET&path="+url.QueryEscape("/invoices/{id}"), &get); err != nil {
		t.Fatal(err)
	}
	if len(get.Results) != 1 || !strings.Contains(get.Results[0].Symbol, "read") {
		t.Fatalf("GET route not resolved: %+v", get)
	}
	var post endpointReply
	if err := e.get("/v1/endpoints?method=POST&path="+url.QueryEscape("/invoices"), &post); err != nil {
		t.Fatal(err)
	}
	if len(post.Results) != 1 || !strings.Contains(post.Results[0].Symbol, "create") {
		t.Fatalf("POST route not resolved: %+v", post)
	}
}
func TestP4AmbiguousBeansRemainAmbiguous(t *testing.T) {
	root := t.TempDir()
	put(t, root, "src/Dispatchers.java", `import org.springframework.stereotype.Service;
interface Dispatcher { void dispatch(); }
@Service class SmsDispatcher implements Dispatcher { public void dispatch() {} }
@Service class EmailDispatcher implements Dispatcher { public void dispatch() {} }
@Service class Sender { Sender(Dispatcher dispatcher) {} }
`)
	e := start(t, root, false)
	var beans beanReply
	if err := e.get("/v1/beans?type="+url.QueryEscape("Dispatcher"), &beans); err != nil {
		t.Fatal(err)
	}
	if len(beans.Results) < 2 {
		t.Fatalf("should find both candidate beans: %+v", beans)
	}
	for _, b := range beans.Results {
		if b.Certainty == "exact" && len(b.InjectedInto) > 0 {
			t.Fatalf("multiple candidates cannot both be exact injection resolution: %+v", b)
		}
	}
}
func TestP4TransactionalMetadataIsNotClaimedToBeRuntimeEffective(t *testing.T) {
	root := t.TempDir()
	put(t, root, "src/InvoiceService.java", `import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
@Service class InvoiceService {
  @Transactional void persistInvoice() {}
  void callSelf() { persistInvoice(); }
}
`)
	e := start(t, root, false)
	var data map[string]any
	if err := e.get("/v1/context?symbol="+url.QueryEscape("InvoiceService.persistInvoice")+"&budget=500", &data); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(data)
	if !strings.Contains(strings.ToLower(string(out)), "transaction") {
		t.Fatalf("declared @Transactional annotation missing from context: %s", out)
	}
	if strings.Contains(string(out), `"transaction_effective":true`) {
		t.Fatalf("cannot assert self-invocation runs through a transaction proxy: %s", out)
	}
}
func TestP4SpringRepositoryInterfaceIsRecognized(t *testing.T) {
	root := t.TempDir()
	put(t, root, "src/InvoiceRepository.java", `import org.springframework.data.jpa.repository.JpaRepository;
interface InvoiceRepository extends JpaRepository<Invoice, Long> {}
class Invoice {}
`)
	e := start(t, root, false)
	r := e.search(t, "InvoiceRepository", "lexical")
	requirePath(t, r, "InvoiceRepository.java")
	var context map[string]any
	if err := e.get("/v1/context?symbol=InvoiceRepository&budget=500", &context); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(context)
	if !strings.Contains(string(payload), "JpaRepository") {
		t.Fatalf("must preserve repository extension relationship: %s", payload)
	}
}

func TestP4QualifierNarrowsSpringInjectionCandidate(t *testing.T) {
	root := t.TempDir()
	put(t, root, "src/Notifiers.java", `import org.springframework.stereotype.Service;
import org.springframework.beans.factory.annotation.Qualifier;
interface AlertSender { void alert(); }
@Service("textAlert") class TextAlert implements AlertSender { public void alert() {} }
@Service("emailAlert") class EmailAlert implements AlertSender { public void alert() {} }
@Service class AlertWorkflow {
  AlertWorkflow(@Qualifier("emailAlert") AlertSender sender) {}
}`)
	e := start(t, root, false)
	var beans beanReply
	if err := e.get("/v1/beans?type=AlertSender", &beans); err != nil {
		t.Fatal(err)
	}
	hasResolved := false
	for _, b := range beans.Results {
		if strings.Contains(b.Symbol, "EmailAlert") && strings.Contains(strings.Join(b.InjectedInto, ","), "AlertWorkflow") {
			hasResolved = b.Certainty == "exact"
		}
	}
	if !hasResolved {
		t.Fatalf("explicit @Qualifier must resolve the matching constructor candidate: %+v", beans)
	}
}
