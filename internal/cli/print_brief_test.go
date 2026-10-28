package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/umuterturk/cxj/internal/ir"
)

func TestPrinterBrief_fullCard(t *testing.T) {
	reply := ir.BriefReply{
		Kind: "spring", Java: "17", SpringBoot: "3.2.0",
		MainFiles: 10, Types: 20, TestClasses: 3,
		Profiles:     []string{"dev"},
		Packages:     []ir.BriefPackage{{Name: "com.example.app", Types: 1}},
		HTTP:         []ir.BriefHTTP{{Method: "GET", Route: "/owners"}},
		Ingress:      []ir.BriefIngress{{Kind: "runner", Symbol: "DemoRunner"}},
		OutboundHTTP: []string{"payments"},
		OutboundDB:   []string{"Owner"},
	}
	var buf bytes.Buffer
	p := &Printer{Out: &buf}
	if p.Brief(reply, Mode{}) != 0 {
		t.Fatal("exit")
	}
	got := buf.String()
	want := ir.FormatBrief(reply) + "\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrinterBrief_omitsEmpty(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Out: &buf}
	p.Brief(ir.BriefReply{Kind: "java", MainFiles: 1, Types: 1}, Mode{})
	got := buf.String()
	if strings.Contains(got, "profiles") || strings.Contains(got, "java=") ||
		strings.Contains(got, "out_") || strings.Contains(got, "http ") {
		t.Fatalf("optional lines leaked: %q", got)
	}
	if !strings.HasPrefix(got, "kind=java\n") {
		t.Fatalf("got %q", got)
	}
}
