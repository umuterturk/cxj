package engine

import (
	"reflect"
	"strings"
	"testing"

	"github.com/umuterturk/cxj/internal/config"
	"github.com/umuterturk/cxj/internal/scan"
)

// The P2 claim is that a command can build fewer sub-indexes and still answer the
// same question. "Still compiles, still returns something" is not evidence for that: a
// trimmed index that quietly returns nothing looks identical to a correct one right up
// until somebody trusts it. So every command here is opened twice, once with the caps
// it declares and once with everything, and the two replies must be identical.
//
// Both directions are checked against an *empty workspace* rather than against a notion
// of "empty": replies carry echo fields (TrailReply.Symbol repeats the query, Status
// carries scan counters), so a hand-written emptiness rule would either forgive a broken
// index or fail on a correct one.

// capsFixture exercises every capability in one small workspace: Java definitions and
// call edges, Spring annotations in both source trees, a config-binding class, yml
// config, a plain text file, and a build file for the versions.
func capsFixture() map[string]string {
	return map[string]string{
		"pom.xml": `<?xml version="1.0"?>
<project>
  <properties>
    <java.version>17</java.version>
    <spring-boot.version>3.2.0</spring-boot.version>
  </properties>
</project>
`,
		"src/main/java/com/example/pay/PaymentController.java": `package com.example.pay;

import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/v1")
public class PaymentController {
    private final PayHandler handler;

    public PaymentController(PayHandler handler) { this.handler = handler; }

    @PostMapping("/payments")
    public String pay(@RequestBody String body) { return handler.handle(body); }
}
`,
		"src/main/java/com/example/pay/PayHandler.java": `package com.example.pay;

import org.springframework.stereotype.Service;

@Service
public class PayHandler {
    private final PayService payService;
    private final PayProperties props;

    public PayHandler(PayService payService, PayProperties props) {
        this.payService = payService;
        this.props = props;
    }

    public String handle(String body) {
        return payService.charge(body, props.getTimeout());
    }
}
`,
		"src/main/java/com/example/pay/PayService.java": `package com.example.pay;

public interface PayService {
    String charge(String body, long timeoutMillis);
}
`,
		"src/main/java/com/example/pay/CardPayService.java": `package com.example.pay;

import org.springframework.stereotype.Service;

@Service
public class CardPayService implements PayService {
    public String charge(String body, long timeoutMillis) { return "ok"; }
}
`,
		// The only thing that makes CapBindings observable: without it, app.pay.*
		// resolves to nothing.
		"src/main/java/com/example/pay/PayProperties.java": `package com.example.pay;

import org.springframework.boot.context.properties.ConfigurationProperties;

@ConfigurationProperties(prefix = "app.pay")
public class PayProperties {
    private long timeout = 3000;

    public long getTimeout() { return timeout; }
}
`,
		"src/main/resources/application.yml":      "app:\n  pay:\n    timeout: 5000\n  name: paymentsvc\n",
		"src/main/resources/application-prod.yml": "app:\n  pay:\n    timeout: 9000\n",
		// Neither config nor Java: only the text index can find this.
		"src/main/resources/notes.txt": "the payflow retry ladder lives here\n",
		// A controller in src/test on purpose: it is what makes the main-only spring
		// rule observable, and what would leak into brief's HTTP section without it.
		"src/test/java/com/example/pay/PaymentControllerTest.java": `package com.example.pay;

import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/test")
public class PaymentControllerTest {
    private final PayHandler handler;

    public PaymentControllerTest(PayHandler handler) { this.handler = handler; }

    @PostMapping("/probe")
    public String probe() { return handler.handle("x"); }
}
`,
	}
}

// emptyFixture is a workspace with nothing to index. Its purpose is to be the answer
// every command gives when it has no data, so that "the trimmed build forgot to build"
// and "there was nothing to say" can be told apart.
func emptyFixture() map[string]string {
	return map[string]string{"README.md": "nothing indexed here\n"}
}

const capHandler = "com.example.pay.PayHandler"

// capsIdentity is one command's question, phrased so that the answer must not depend on
// which sub-indexes happened to be built.
type capsIdentity struct {
	cmd      string
	ask      string
	call     func(*Engine) any
	baseline Capabilities // zero means CapsAll
}

func identityCases() []capsIdentity {
	return []capsIdentity{
		{"brief", "card", func(e *Engine) any { return e.Brief() }, briefBaselineCaps},
		{"find", "type name", func(e *Engine) any { return e.Search("PayHandler", 20).Results }, 0},
		{"find", "mid-name", func(e *Engine) any { return e.Search("Controller", 20).Results }, 0},
		{"find", "text-only token", func(e *Engine) any { return e.Search("payflow", 20).Results }, 0},
		{"show", "symbol", func(e *Engine) any { return e.Show(capHandler, 80) }, 0},
		{"outline", "symbol", func(e *Engine) any { return e.Outline(capHandler) }, 0},
		{"outline", "path", func(e *Engine) any {
			return e.Outline("src/main/java/com/example/pay/PayHandler.java")
		}, 0},
		{"map", "packages", func(e *Engine) any { return e.Map() }, 0},
		{"trail", "type", func(e *Engine) any { return e.Trail(capHandler) }, 0},
		{"trail", "method", func(e *Engine) any { return e.Trail(capHandler + ".handle") }, 0},
		{"references", "method", func(e *Engine) any { return e.References(capHandler + ".handle") }, 0},
		{"callers", "method", func(e *Engine) any { return e.Callers(capHandler + ".handle") }, 0},
		{"callees", "method", func(e *Engine) any { return e.Callees(capHandler + ".handle") }, 0},
		{"endpoints", "all", func(e *Engine) any { return e.Endpoints("", "") }, 0},
		{"endpoints", "filtered", func(e *Engine) any { return e.Endpoints("POST", "/v1/payments") }, 0},
		// Beans("") is not a case: beanProvides matches a type name, so the empty
		// query answers nothing in any workspace and would compare two blanks.
		{"beans", "by type", func(e *Engine) any { return e.Beans("PayService") }, 0},
		{"context", "symbol", func(e *Engine) any { return e.Context(capHandler, 200) }, 0},
		{"keys", "names", func(e *Engine) any { return e.Keys() }, 0},
		// Config("") is not a case: the CLI rejects an empty --key before opening, so
		// there is no such question to keep identical.
		{"config", "one key", func(e *Engine) any { return e.Config("app.pay.timeout", "") }, 0},
		{"config", "profiled", func(e *Engine) any { return e.Config("app.pay.timeout", "prod") }, 0},
		{"status", "counters", func(e *Engine) any {
			s := e.Status()
			// Heap size depends on what the process has allocated: the one field in
			// the payload legitimately allowed to differ between two identical opens.
			s.GoHeapAllocBytes = 0
			return s
		}, 0},
	}
}

// briefBaselineCaps is CapsAll with the main-only spring rule kept, because brief is the
// one command where a capability changes what is *said* rather than what it costs: its
// HTTP/ingress/outbound sections describe the deployed app, so spring facts come from
// main sources only. TestBriefSpringIsMainOnly proves the distinction is real.
const briefBaselineCaps = (CapsAll &^ CapSpring) | CapSpringMain

func baselineOf(c Capabilities) Capabilities {
	if c == 0 {
		return CapsAll
	}
	return c
}

func TestTrimmedCapsAnswerIdentically(t *testing.T) {
	root := writeTree(t, capsFixture())
	blank := writeTree(t, emptyFixture())
	for _, scope := range []scan.Scope{scan.ScopeAll, scan.ScopeMain} {
		for _, tc := range identityCases() {
			t.Run(tc.cmd+"/"+tc.ask+"/"+scopeName(scope), func(t *testing.T) {
				trimmed, err := Open(root, Options{Caps: CapsFor(tc.cmd), Scope: scope})
				if err != nil {
					t.Fatal(err)
				}
				full, err := Open(root, Options{Caps: baselineOf(tc.baseline), Scope: scope})
				if err != nil {
					t.Fatal(err)
				}
				nothing, err := Open(blank, Options{Caps: baselineOf(tc.baseline), Scope: scope})
				if err != nil {
					t.Fatal(err)
				}
				want, void := tc.call(full), tc.call(nothing)
				if reflect.DeepEqual(want, void) {
					t.Fatalf("the full build answers %s/%s no differently than it answers an empty "+
						"workspace, so the fixture never exercised this command and any two trimmed "+
						"answers would have agreed anyway", tc.cmd, tc.ask)
				}
				if got := tc.call(trimmed); !reflect.DeepEqual(got, want) {
					t.Errorf("caps changed the answer for %s/%s\n trimmed(%s): %#v\n    full(%s): %#v",
						tc.cmd, tc.ask, capsName(CapsFor(tc.cmd)), got, capsName(baselineOf(tc.baseline)), want)
				}
			})
		}
	}
}

// TestNoCapabilitiesLooksLikeAnEmptyWorkspace: a command that misdeclares its caps must
// answer nothing, not crash and not half-answer. This is why every index is built even
// when nothing was extracted into it: a nil index panics on first use, and a panic in a
// one-shot CLI reads as a broken tool rather than as a missing line in one table.
func TestNoCapabilitiesLooksLikeAnEmptyWorkspace(t *testing.T) {
	root := writeTree(t, capsFixture())
	blank := writeTree(t, emptyFixture())
	none, err := Open(root, Options{Caps: 0, Scope: scan.ScopeAll})
	if err != nil {
		t.Fatal(err)
	}
	void, err := Open(blank, Options{Caps: CapsAll, Scope: scan.ScopeAll})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range identityCases() {
		if tc.cmd == "status" {
			// The one command whose whole answer is the scan. It declares no
			// capabilities on purpose: `cxj status` reports how many files are there,
			// which needs a walk and no parsing at all.
			continue
		}
		got, want := tc.call(none), tc.call(void)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s/%s answered with no capabilities built\n got:  %#v\n want: %#v", tc.cmd, tc.ask, got, want)
		}
	}
}

// TestBriefSpringIsMainOnly pins the single exception in the table above. If spring over
// all sources gave brief the same sections, the main-only capability would be decoration
// and CapsBrief could simply use CapSpring.
func TestBriefSpringIsMainOnly(t *testing.T) {
	root := writeTree(t, capsFixture())
	lean, err := Open(root, Options{Caps: CapsBrief, Scope: scan.ScopeAll})
	if err != nil {
		t.Fatal(err)
	}
	broad, err := Open(root, Options{Caps: (CapsAll &^ CapSpringMain) | CapSpring, Scope: scan.ScopeAll})
	if err != nil {
		t.Fatal(err)
	}
	leanBrief, broadBrief := lean.Brief(), broad.Brief()
	if len(leanBrief.HTTP) == 0 {
		t.Fatal("brief lost its own main-source route")
	}
	for _, h := range leanBrief.HTTP {
		if h.Route == "/test/probe" {
			t.Fatal("brief advertised a route that only exists in a test class")
		}
	}
	var sawTestRoute bool
	for _, h := range broadBrief.HTTP {
		if h.Route == "/test/probe" {
			sawTestRoute = true
		}
	}
	if !sawTestRoute {
		t.Fatal("spring over all sources did not pick up the test controller either, so the fixture" +
			" never exercised the distinction: lean and broad agreeing would have meant nothing")
	}
}

// TestCapsNeverAskForContradictorySpring: CapSpring (every in-scope file) and
// CapSpringMain (main sources only) are the same parse with different reach. Setting both
// would make the answer depend on the order of a switch.
func TestCapsNeverAskForContradictorySpring(t *testing.T) {
	for cmd, caps := range commandCaps {
		if caps.Has(CapSpring) && caps.Has(CapSpringMain) {
			t.Errorf("%s sets both CapSpring and CapSpringMain", cmd)
		}
	}
	for _, unknown := range []string{"", "no-such-command", "brief2"} {
		if CapsFor(unknown) != CapsAll {
			t.Errorf("CapsFor(%q) should be everything: slow is fixable, an empty answer is a wrong one", unknown)
		}
		if KnowsCommand(unknown) {
			t.Errorf("KnowsCommand(%q) lied", unknown)
		}
	}
}

// TestSpringOnlyCommandsSkipJava is the sharpest edge of the table: endpoints, beans and
// context are answered from Spring annotations, which carry their own file, line and
// symbol, so the Java extractor is pure waste there.
func TestSpringOnlyCommandsSkipJava(t *testing.T) {
	root := writeTree(t, capsFixture())
	eng, err := Open(root, Options{Caps: CapsFor("endpoints"), Scope: scan.ScopeAll})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(eng.facts); got != 0 {
		t.Errorf("facts=%d, endpoints asked for a Java parse", got)
	}
	if got := len(eng.syms.Search("PayHandler", 5)); got != 0 {
		t.Errorf("symbol index=%d under CapsSpring", got)
	}
}

// TestConfigCommandsSkipJava: `keys` used to parse every Java file three times in order
// to print config key names.
func TestConfigCommandsSkipJava(t *testing.T) {
	root := writeTree(t, capsFixture())
	eng, err := Open(root, Options{Caps: CapsFor("keys"), Scope: scan.ScopeAll})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(eng.facts); got != 0 {
		t.Errorf("facts=%d, keys asked for a Java parse", got)
	}
	if got := textHits(eng); got != 0 {
		t.Errorf("keys built a text index (%d probe hits)", got)
	}
}

// TestSymbolCommandsSkipSpringAndConfig is the other half: the navigational commands are
// the ones an agent runs most, and none of them reads an annotation or a yml file.
func TestSymbolCommandsSkipSpringAndConfig(t *testing.T) {
	root := writeTree(t, capsFixture())
	for _, cmd := range []string{"show", "outline", "map", "references", "callers", "callees"} {
		eng, err := Open(root, Options{Caps: CapsFor(cmd), Scope: scan.ScopeAll})
		if err != nil {
			t.Fatal(err)
		}
		if got := len(eng.spring.Beans("PayService")); got != 0 {
			t.Errorf("%s: spring index has %d beans, it reads no annotations", cmd, got)
		}
		if got := textHits(eng); got != 0 {
			t.Errorf("%s: text search returned %d hits, it does not search text", cmd, got)
		}
		if len(eng.facts) == 0 {
			t.Errorf("%s: no Java facts, so it cannot answer anything", cmd)
		}
	}
}

// TestCapsFixtureIsLive: every "index X stayed empty" assertion above is only meaningful
// if the same fixture fills X when the capability is on.
func TestCapsFixtureIsLive(t *testing.T) {
	root := writeTree(t, capsFixture())
	full, err := Open(root, Options{Caps: CapsAll, Scope: scan.ScopeAll})
	if err != nil {
		t.Fatal(err)
	}
	if len(full.facts) == 0 {
		t.Fatal("no Java facts: the fixture has no Java in scope")
	}
	if len(full.spring.Beans("PayService")) == 0 {
		t.Fatal("no beans for PayService: the fixture has no spring annotations")
	}
	if textHits(full) == 0 {
		t.Fatal("the text probe matches nothing: every text assertion above is vacuous")
	}
	if len(full.Keys().Keys) == 0 {
		t.Fatal("no config keys: the fixture has no readable config")
	}
	r, ok := full.Config("app.pay.timeout", "").(config.Reply)
	if !ok || r.Resolution == "" || r.Resolution == "unknown" {
		t.Fatalf("app.pay.timeout unresolved (%#v): CapBindings has nothing to prove", full.Config("app.pay.timeout", ""))
	}
	if full.javaVer == "" || full.bootVer == "" {
		t.Fatalf("versions=%q/%q: CapVersions has nothing to prove", full.javaVer, full.bootVer)
	}
}

// textHits probes the text index with a word certain to appear in the fixture's non-Java
// files, so zero means the index is genuinely empty rather than merely uninterested in
// the query.
// TestBriefSkipsTheSymbolIndex: brief is the first call an agent makes and it reports
// counts read straight off the per-file facts, never a symbol lookup. Building the index
// it does not query cost ~6% of brief's cpu, which is how P2 briefly made the tool slower
// than it started.
func TestBriefSkipsTheSymbolIndex(t *testing.T) {
	root := writeTree(t, capsFixture())
	eng, err := Open(root, Options{Caps: CapsBrief, Scope: scan.ScopeAll})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(eng.syms.Search("PayHandler", 5)); got != 0 {
		t.Errorf("brief built a symbol index (%d hits); it never queries one", got)
	}
	if len(eng.facts) == 0 {
		t.Fatal("brief has no facts, so its counts are all zero")
	}
	if got := eng.Brief().Types; got == 0 {
		t.Error("Types=0, so brief lost the count it does need")
	}
}

func textHits(e *Engine) int { return len(e.text.Search("the", 50)) }

func scopeName(s scan.Scope) string {
	switch s {
	case scan.ScopeMain:
		return "main"
	case scan.ScopeTest:
		return "test"
	default:
		return "all"
	}
}

func capsName(c Capabilities) string {
	var on []string
	for _, w := range []struct {
		bit  Capabilities
		name string
	}{
		{CapJava, "java"}, {CapSpring, "spring"}, {CapSpringMain, "springMain"},
		{CapConfig, "config"}, {CapBindings, "bindings"}, {CapText, "text"}, {CapVersions, "versions"},
	} {
		if c.Has(w.bit) {
			on = append(on, w.name)
		}
	}
	if len(on) == 0 {
		return "none"
	}
	return strings.Join(on, "|")
}
