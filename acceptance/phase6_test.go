package acceptance

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestP6CreateModifyDeleteAndRenameAreVisible(t *testing.T) {
	root := t.TempDir()
	put(t, root, "LiveAlpha.java", "class LiveAlpha {}\n")
	e := open(t, root)
	requirePath(t, e.search(t, "LiveAlpha"), "LiveAlpha.java")

	put(t, root, "LiveAlpha.java", "class LiveBeta {}\n")
	if !hasPath(e.search(t, "LiveBeta"), "LiveAlpha.java") {
		t.Fatal("modified Java symbol not visible on next run")
	}
	if hasPath(e.search(t, "LiveAlpha"), "LiveAlpha.java") {
		t.Fatal("old symbol survived edit")
	}

	if err := os.Rename(filepath.Join(root, "LiveAlpha.java"), filepath.Join(root, "Renamed.java")); err != nil {
		t.Fatal(err)
	}
	if !hasPath(e.search(t, "LiveBeta"), "Renamed.java") {
		t.Fatal("renamed path not visible on next run")
	}

	if err := os.Remove(filepath.Join(root, "Renamed.java")); err != nil {
		t.Fatal(err)
	}
	if hasPath(e.search(t, "LiveBeta"), "Renamed.java") {
		t.Fatal("deleted symbol still visible")
	}
}

func TestP6UpdatesExcludedFilesWithoutLeaks(t *testing.T) {
	root := t.TempDir()
	put(t, root, "Generated/Ignore.java", "class Ignore {}\n")
	e := open(t, root, "Generated/**")
	put(t, root, "Generated/NewDecoy.java", "class NewDecoy {}\n")
	requireNoPath(t, e.search(t, "NewDecoy"), "NewDecoy.java")
}

func TestP6CLIFindReturnsRealSource(t *testing.T) {
	root := t.TempDir()
	put(t, root, "ToolSmoke.java", "class ToolSmoke {}\n")
	cmd := exec.Command(binary(t), "find", "ToolSmoke", "--root", root, "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cx find failed: %v; output=%s", err, out)
	}
	if !bytes.Contains(out, []byte("ToolSmoke.java")) {
		t.Fatalf("cx find did not return real source: %s", out)
	}
	var reply searchReply
	if err := json.Unmarshal(out, &reply); err != nil {
		t.Fatalf("cx find --json must emit search JSON: %v; output=%s", err, out)
	}
	if !hasPath(reply.Results, "ToolSmoke.java") {
		t.Fatalf("expected ToolSmoke.java in results: %#v", reply.Results)
	}
}
