package version

import "testing"

func TestLine_devDefault(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })
	Version = "dev"
	if got := Line(); got != "cxj dev" {
		t.Fatalf("got %q", got)
	}
}

func TestCurrent_addsVPrefix(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })
	Version = "0.2.0"
	if got := Current(); got != "v0.2.0" {
		t.Fatalf("got %q", got)
	}
	Version = "v0.2.0"
	if got := Current(); got != "v0.2.0" {
		t.Fatalf("got %q", got)
	}
}

func TestSame(t *testing.T) {
	if !Same("v0.2.0", "0.2.0") {
		t.Fatal("expected same")
	}
	if Same("v0.2.0", "v0.3.0") {
		t.Fatal("expected different")
	}
	if Same("dev", "v0.2.0") {
		t.Fatal("dev is never latest")
	}
}
