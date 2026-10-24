package acceptance

import (
	"encoding/json"
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
	e := open(t, root)
	get := e.endpoints(t, "GET", "/invoices/{id}")
	if len(get.Results) != 1 || !strings.Contains(get.Results[0].Symbol, "read") {
		t.Fatalf("GET route not resolved: %+v", get)
	}
	post := e.endpoints(t, "POST", "/invoices")
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
	e := open(t, root)
	beans := e.beans(t, "Dispatcher")
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
	e := open(t, root)
	data := e.context(t, "InvoiceService.persistInvoice")
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
	e := open(t, root)
	r := e.search(t, "InvoiceRepository")
	requirePath(t, r, "InvoiceRepository.java")
	payload, _ := json.Marshal(e.context(t, "InvoiceRepository"))
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
	e := open(t, root)
	beans := e.beans(t, "AlertSender")
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
