package engine

import (
	"strings"
	"testing"
)

func TestStaticExamples_versionUpdate(t *testing.T) {
	ver := StaticExamples("version")
	if len(ver) != 1 || ver[0].Cmd != "cxj version" {
		t.Fatalf("%+v", ver)
	}
	up := StaticExamples("update")
	if len(up) != 2 || up[0].Cmd != "cxj update --check" {
		t.Fatalf("%+v", up)
	}
}

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
