package engine

import "testing"

func TestParsePomVersions(t *testing.T) {
	java, boot := parsePomVersions(`
<project>
  <parent>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-parent</artifactId>
    <version>3.2.0</version>
  </parent>
  <properties>
    <java.version>17</java.version>
  </properties>
</project>
`)
	if java != "17" || boot != "3.2.0" {
		t.Fatalf("java=%q boot=%q", java, boot)
	}
	java, boot = parsePomVersions(`
<project>
  <properties>
    <maven.compiler.source>1.8</maven.compiler.source>
    <spring-boot.version>2.7.18</spring-boot.version>
  </properties>
</project>
`)
	if java != "8" || boot != "2.7.18" {
		t.Fatalf("java=%q boot=%q", java, boot)
	}
}

func TestParseGradleVersions(t *testing.T) {
	java, boot := parseGradleVersions(`
plugins {
  id 'org.springframework.boot' version '3.1.5'
}
java {
  toolchain { languageVersion = JavaLanguageVersion.of(21) }
}
`)
	if java != "21" || boot != "3.1.5" {
		t.Fatalf("java=%q boot=%q", java, boot)
	}
	java, boot = parseGradleVersions(`
id("org.springframework.boot") version "3.3.1"
sourceCompatibility = '17'
`)
	if java != "17" || boot != "3.3.1" {
		t.Fatalf("java=%q boot=%q", java, boot)
	}
}

func TestParseBuildVersions_ignoresOtherFiles(t *testing.T) {
	j, b := parseBuildVersions("application.yml", []byte("java.version: 17"))
	if j != "" || b != "" {
		t.Fatalf("java=%q boot=%q", j, b)
	}
}

func TestPickVersions_prefersShallower(t *testing.T) {
	java, boot := pickVersions([]versionHit{
		{depth: 2, java: "11", boot: "2.0.0"},
		{depth: 0, java: "17", boot: ""},
		{depth: 1, java: "", boot: "3.2.0"},
	})
	if java != "17" || boot != "3.2.0" {
		t.Fatalf("java=%q boot=%q", java, boot)
	}
}
