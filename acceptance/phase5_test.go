package acceptance

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestP5YamlAndPropertiesDefinitionsKeepProfileSource(t *testing.T) {
	root := t.TempDir()
	put(t, root, "src/main/resources/application.yml", "billing:\n  provider:\n    timeout: 1500\n")
	put(t, root, "src/main/resources/application-prod.properties", "billing.provider.timeout=3500\n")
	e := open(t, root)
	c := e.config(t, "billing.provider.timeout", "prod")
	if len(c.Definitions) != 2 {
		t.Fatalf("need both definitions and origins, got %+v", c)
	}
	if c.RepositoryValue != "3500" || c.Resolution != "known" {
		t.Fatalf("wrong profile repository value: %+v", c)
	}
}

func TestP5PlaceholderIsNotInventedAsRuntimeValue(t *testing.T) {
	root := t.TempDir()
	put(t, root, "application-prod.yml", "billing:\n  gateway:\n    uri: ${PAYMENT_PRIVATE_URL}\n")
	e := open(t, root)
	c := e.config(t, "billing.gateway.uri", "prod")
	if c.Resolution != "external" {
		t.Fatalf("environment placeholder must be marked external: %+v", c)
	}
	if strings.HasPrefix(c.RepositoryValue, "http") {
		t.Fatalf("runtime endpoint is unknown: %+v", c)
	}
}

func TestP5ConfigurationPropertyIsLinkedToJavaBinding(t *testing.T) {
	root := t.TempDir()
	put(t, root, "application.properties", "billing.max-retries=4\n")
	put(t, root, "BillingSettings.java", `import org.springframework.boot.context.properties.ConfigurationProperties;
@ConfigurationProperties(prefix="billing")
record BillingSettings(int maxRetries) {}
`)
	e := open(t, root)
	c := e.config(t, "billing.max-retries", "")
	if len(c.Bindings) == 0 || !strings.Contains(strings.Join(c.Bindings, " "), "BillingSettings") {
		t.Fatalf("Spring relaxed binding must link configuration to Java record: %+v", c)
	}
}

func TestP5SecretValuesNeverAppearInResults(t *testing.T) {
	root := t.TempDir()
	secret := "super-private-token-9592-do-not-return"
	put(t, root, "application.yml", "security:\n  api-token: "+secret+"\n")
	e := open(t, root)
	c := e.config(t, "security.api-token", "")
	payload, _ := json.Marshal(c)
	if strings.Contains(string(payload), secret) {
		t.Fatalf("secret leaked from config endpoint: %s", payload)
	}
	found := e.search(t, "security.api-token")
	payload, _ = json.Marshal(found)
	if strings.Contains(string(payload), secret) {
		t.Fatalf("secret leaked from search snippet: %s", payload)
	}
}

func TestP5ExcludedConfigStaysInvisible(t *testing.T) {
	root := t.TempDir()
	put(t, root, "secure/application-prod.yml", "billing:\n  hidden-setting: 424242\n")
	e := open(t, root, "secure/**")
	c := e.config(t, "billing.hidden-setting", "prod")
	if len(c.Definitions) != 0 {
		t.Fatalf("excluded config was analyzed: %+v", c)
	}
	requireNoPath(t, e.search(t, "424242"), "application-prod.yml")
}

func TestP5ValueAnnotationCreatesConfigReference(t *testing.T) {
	root := t.TempDir()
	put(t, root, "application.properties", "cache.expire-seconds=120\n")
	put(t, root, "CacheConfig.java", `import org.springframework.beans.factory.annotation.Value;
class CacheConfig {
  @Value("${cache.expire-seconds}") private int ttl;
}`)
	e := open(t, root)
	c := e.config(t, "cache.expire-seconds", "")
	if !strings.Contains(strings.Join(c.Bindings, " "), "CacheConfig") {
		t.Fatalf("@Value reference was not connected to config definition: %+v", c)
	}
}

func TestP5ConditionalPropertyMustNotClaimRuntimeBeanActivation(t *testing.T) {
	root := t.TempDir()
	put(t, root, "application.yml", "notifications:\n  enabled: ${NOTIFY_SWITCH}\n")
	put(t, root, "NotificationBean.java", `import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.stereotype.Service;
@Service
@ConditionalOnProperty(prefix="notifications",name="enabled",havingValue="true")
class NotificationBean {}`)
	e := open(t, root)
	c := e.config(t, "notifications.enabled", "")
	if c.Resolution != "external" {
		t.Fatalf("external conditional value cannot be called known: %+v", c)
	}
	if !strings.Contains(strings.Join(c.Bindings, " "), "NotificationBean") {
		t.Fatalf("conditional bean relationship not indexed: %+v", c)
	}
}

func TestP5LocalConfigImportOverridesImporter(t *testing.T) {
	root := t.TempDir()
	put(t, root, "src/main/resources/application.yml", `spring:
  config:
    import: "classpath:defaults.yml"
queue:
  size: 25
`)
	put(t, root, "src/main/resources/defaults.yml", `queue:
  size: 70
`)
	e := open(t, root)
	c := e.config(t, "queue.size", "")
	if c.RepositoryValue != "70" || c.Resolution != "known" {
		t.Fatalf("supported local import should override importer: %+v", c)
	}
}

func TestP5MultiDocumentYamlProfileSelection(t *testing.T) {
	root := t.TempDir()
	put(t, root, "src/main/resources/application.yml", `limits:
  batch: 100
---
spring:
  config:
    activate:
      on-profile: prod
limits:
  batch: 750
`)
	e := open(t, root)
	prod := e.config(t, "limits.batch", "prod")
	if prod.RepositoryValue != "750" {
		t.Fatalf("prod multi-document value wrong: %+v", prod)
	}
	dev := e.config(t, "limits.batch", "dev")
	if dev.RepositoryValue != "100" {
		t.Fatalf("inactive prod document applied to dev: %+v", dev)
	}
}
