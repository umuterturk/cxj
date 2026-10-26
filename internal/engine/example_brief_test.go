package engine

import (
	"strings"
	"testing"
)

func TestExamples_briefWhy(t *testing.T) {
	eng, err := Open(writeTree(t, map[string]string{
		"src/main/java/com/example/App.java": "package com.example; class App {}",
	}), Options{})
	if err != nil {
		t.Fatal(err)
	}
	ex := eng.Examples("brief")
	if len(ex) == 0 || ex[0].Cmd != "cxj brief" {
		t.Fatalf("examples=%+v", ex)
	}
	if !strings.Contains(ex[0].Why, "kind") || !strings.Contains(ex[0].Why, "outbound") {
		t.Fatalf("why=%q", ex[0].Why)
	}
}
