package engine

import (
	"regexp"
	"sort"
	"strings"
)

type versionHit struct {
	depth int
	java  string
	boot  string
}

func parseBuildVersions(base string, data []byte) (java, boot string) {
	switch base {
	case "pom.xml":
		return parsePomVersions(string(data))
	case "build.gradle", "build.gradle.kts":
		return parseGradleVersions(string(data))
	default:
		return "", ""
	}
}

func pickVersions(hits []versionHit) (java, boot string) {
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].depth < hits[j].depth })
	for _, h := range hits {
		if java == "" {
			java = h.java
		}
		if boot == "" {
			boot = h.boot
		}
		if java != "" && boot != "" {
			break
		}
	}
	return java, boot
}

func parsePomVersions(s string) (java, boot string) {
	java = firstSub(s, `(?s)<java\.version>\s*([^<]+?)\s*</java\.version>`)
	if java == "" {
		java = firstSub(s, `(?s)<maven\.compiler\.release>\s*([^<]+?)\s*</maven\.compiler\.release>`)
	}
	if java == "" {
		java = firstSub(s, `(?s)<maven\.compiler\.source>\s*([^<]+?)\s*</maven\.compiler\.source>`)
	}
	java = normalizeJavaVersion(java)

	if parent := firstSub(s, `(?s)<parent>(.*?)</parent>`); strings.Contains(parent, "org.springframework.boot") {
		boot = firstSub(parent, `(?s)<version>\s*([^<]+?)\s*</version>`)
	}
	if boot == "" {
		boot = firstSub(s, `(?s)<spring-boot\.version>\s*([^<]+?)\s*</spring-boot\.version>`)
	}
	if boot == "" {
		boot = firstSub(s, `(?s)<spring\.boot\.version>\s*([^<]+?)\s*</spring\.boot\.version>`)
	}
	return strings.TrimSpace(java), strings.TrimSpace(boot)
}

func parseGradleVersions(s string) (java, boot string) {
	boot = firstSub(s, `id\s*\(?\s*['"]org\.springframework\.boot['"]\s*\)?\s+version\s+['"]([^'"]+)['"]`)
	if boot == "" {
		boot = firstSub(s, `springBootVersion\s*=\s*['"]([^'"]+)['"]`)
	}
	if boot == "" {
		boot = firstSub(s, `org\.springframework\.boot:spring-boot-dependencies:([0-9][0-9A-Za-z._-]*)`)
	}

	java = firstSub(s, `JavaLanguageVersion\.of\(\s*(\d+)\s*\)`)
	if java == "" {
		java = firstSub(s, `JavaVersion\.VERSION_(\d+)`)
	}
	if java == "" {
		java = firstSub(s, `sourceCompatibility\s*=\s*['"]?(?:1\.)?(\d+)['"]?`)
	}
	return normalizeJavaVersion(java), strings.TrimSpace(boot)
}

func normalizeJavaVersion(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "1.") && len(v) > 2 {
		return v[2:]
	}
	return v
}

func firstSub(s, pat string) string {
	re := regexp.MustCompile(pat)
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}
