package ir

import (
	"strings"
	"testing"
)

func TestFormatBrief_fullCard(t *testing.T) {
	got := FormatBrief(BriefReply{
		Kind: "spring", Java: "17", SpringBoot: "3.2.0",
		MainFiles: 10, Types: 20, TestClasses: 3,
		Profiles: []string{"dev", "prod"},
		Packages: []BriefPackage{
			{Name: "com.example.app", Scope: "main", Types: 1},
			{Name: "com.example.app.owner", Scope: "main", Types: 5},
		},
		HTTP: []BriefHTTP{
			{Method: "GET", Route: "/owners"},
			{Method: "POST", Route: "/owners"},
		},
		Ingress: []BriefIngress{
			{Kind: "runner", Symbol: "DemoRunner"},
			{Kind: "scheduled", Symbol: "Jobs.cleanup"},
		},
		OutboundHTTP: []string{"payments", "billing"},
		OutboundDB:   []string{"Owner", "Vet"},
	})
	want := strings.Join([]string{
		"kind=spring java=17 spring_boot=3.2.0",
		"main_files=10 types=20 test_classes=3",
		"profiles dev,prod",
		"pkg com.example.app 1",
		"pkg com.example.app.owner 5",
		"http GET /owners",
		"http POST /owners",
		"runner DemoRunner",
		"scheduled Jobs.cleanup",
		"out_http payments,billing",
		"out_db Owner,Vet",
	}, "\n")
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatBrief_omitsEmptyOptionalLines(t *testing.T) {
	got := FormatBrief(BriefReply{Kind: "java", MainFiles: 2, Types: 3})
	want := "kind=java\nmain_files=2 types=3 test_classes=0"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(got, "profiles") || strings.Contains(got, "pkg ") ||
		strings.Contains(got, "http ") || strings.Contains(got, "out_") {
		t.Fatalf("optional lines leaked: %q", got)
	}
}
