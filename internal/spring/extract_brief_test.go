package spring

import (
	"reflect"
	"testing"
)

func extractFile(t *testing.T, src string) Facts {
	t.Helper()
	ext := NewExtractor()
	t.Cleanup(ext.Close)
	return ext.Extract("src/main/java/X.java", []byte(src))
}

func TestExtract_httpMappings(t *testing.T) {
	f := extractFile(t, `
package com.example.app.owner;
@RestController
@RequestMapping("/owners")
class OwnerController {
  @GetMapping
  Object list() { return null; }
  @PostMapping("/{id}")
  Object one() { return null; }
}
`)
	if len(f.Endpoints) != 2 {
		t.Fatalf("endpoints=%+v", f.Endpoints)
	}
	got := map[string]string{}
	for _, e := range f.Endpoints {
		got[e.HTTPMethod] = e.Path
	}
	if got["GET"] != "/owners" || got["POST"] != "/owners/{id}" {
		t.Fatalf("got %v", got)
	}
}

func TestExtract_ingressKinds(t *testing.T) {
	f := extractFile(t, `
package com.example.app;
class DemoRunner implements CommandLineRunner {}
class Jobs {
  @Scheduled(cron = "0 * * * * *")
  void cleanup() {}
  @KafkaListener(topics = "t")
  void onMsg() {}
}
`)
	kinds := map[string]int{}
	for _, ep := range f.Entrypoints {
		kinds[ep.Kind]++
	}
	if kinds["runner"] != 1 || kinds["scheduled"] != 1 || kinds["kafka"] != 1 {
		t.Fatalf("kinds=%v entry=%+v", kinds, f.Entrypoints)
	}
}

func TestExtract_mainIsNotBoot(t *testing.T) {
	f := extractFile(t, `
package com.example;
class App {
  public static void main(String[] args) {}
}
`)
	for _, ep := range f.Entrypoints {
		if ep.Kind == "boot" {
			t.Fatalf("plain main marked boot: %+v", ep)
		}
		if ep.Kind != "main" {
			t.Fatalf("kind=%q", ep.Kind)
		}
	}
}

func TestExtract_feignAndRestTemplate(t *testing.T) {
	f := extractFile(t, `
package com.example.app;
@FeignClient(url = "http://x", name = "payments")
interface PaymentsClient {}
class Gateway {
  private RestTemplate restTemplate;
  Gateway(WebClient webClient) {}
}
`)
	idx := Build([]Facts{f})
	got := idx.OutboundHTTP()
	want := []string{"Gateway", "payments"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("out_http=%v want %v", got, want)
	}
}

func TestExtract_restClientEnclosingAndQualifier(t *testing.T) {
	f := extractFile(t, `
package com.example.app;
class ZipRestAdapter {
  private RestClient restClient;
}
class Named {
  Named(@Qualifier("kyc") RestClient client) {}
}
`)
	idx := Build([]Facts{f})
	got := idx.OutboundHTTP()
	want := []string{"ZipRestAdapter", "kyc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("out_http=%v want %v", got, want)
	}
}

func TestExtract_httpExchange(t *testing.T) {
	f := extractFile(t, `
package com.example.app;
@HttpExchange(contentType = "application/json")
interface ZipHttpClient {
  @PostExchange("/psp/v5/checkouts/")
  Object create();
}
@HttpExchange
interface KycHttpClient {}
`)
	idx := Build([]Facts{f})
	got := idx.OutboundHTTP()
	want := []string{"KycHttpClient", "ZipHttpClient"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("out_http=%v want %v", got, want)
	}
}

func TestExtract_repoEntity(t *testing.T) {
	f := extractFile(t, `
package com.example.app;
interface OwnerRepository extends JpaRepository<Owner, Long> {}
interface VetRepository extends CrudRepository<Vet, Integer> {}
`)
	if len(f.Repositories) != 2 {
		t.Fatalf("repos=%+v", f.Repositories)
	}
	got := map[string]string{}
	for _, r := range f.Repositories {
		got[r.Symbol] = r.Entity
	}
	if got["com.example.app.OwnerRepository"] != "Owner" || got["com.example.app.VetRepository"] != "Vet" {
		t.Fatalf("entities=%v", got)
	}
	idx := Build([]Facts{f})
	if !reflect.DeepEqual(idx.OutboundDB(), []string{"Owner", "Vet"}) {
		t.Fatalf("out_db=%v", idx.OutboundDB())
	}
}

func TestExtract_feignValuePositional(t *testing.T) {
	f := extractFile(t, `
package com.example.app;
@FeignClient("billing")
interface BillingClient {}
`)
	idx := Build([]Facts{f})
	if !reflect.DeepEqual(idx.OutboundHTTP(), []string{"billing"}) {
		t.Fatalf("got %v", idx.OutboundHTTP())
	}
}
