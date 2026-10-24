package config

import "strings"

// Redacted is the placeholder used for secret values.
const Redacted = "[REDACTED]"

var secretKeyParts = []string{
	"password", "secret", "token", "apikey", "api-key", "api_key",
	"private-key", "private_key", "credential", "passwd", "auth-key",
}

// IsSecretKey reports whether a config key looks sensitive.
func IsSecretKey(key string) bool {
	k := strings.ToLower(key)
	for _, p := range secretKeyParts {
		if strings.Contains(k, p) {
			return true
		}
	}
	return false
}

// RedactValue returns Redacted for secret keys.
func RedactValue(key, value string) string {
	if IsSecretKey(key) {
		return Redacted
	}
	return value
}

// RedactText replaces secret-looking assignments in free text snippets.
func RedactText(text string) string {
	// Conservative: if line contains a secret key fragment and a value after :, redact value-ish tails.
	lower := strings.ToLower(text)
	for _, p := range secretKeyParts {
		if !strings.Contains(lower, p) {
			continue
		}
		// Replace long token-like substrings after the key occurrence.
		return redactAfterKey(text, p)
	}
	return text
}

func redactAfterKey(text, keyPart string) string {
	lower := strings.ToLower(text)
	idx := strings.Index(lower, keyPart)
	if idx < 0 {
		return text
	}
	// Find ':' or '=' after key
	rest := text[idx:]
	sep := strings.IndexAny(rest, ":=")
	if sep < 0 {
		return text
	}
	prefix := text[:idx+sep+1]
	return prefix + " " + Redacted
}
