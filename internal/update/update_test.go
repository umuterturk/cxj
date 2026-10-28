package update

import (
	"archive/zip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/umuterturk/cxj/internal/version"
)

func TestAssetName(t *testing.T) {
	name, err := AssetName("darwin", "arm64")
	if err != nil || name != "cxj-macos-arm64.zip" {
		t.Fatalf("got %q %v", name, err)
	}
	if _, err := AssetName("plan9", "arm"); err == nil {
		t.Fatal("expected error")
	}
}

func TestCheck_latest(t *testing.T) {
	old := version.Version
	t.Cleanup(func() { version.Version = old })
	version.Version = "v0.2.0"

	var downloaded bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/umuterturk/cxj/releases/latest":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v0.3.0",
				"assets": []map[string]string{{
					"name":                 "cxj-macos-arm64.zip",
					"browser_download_url": "http://" + r.Host + "/cxj-macos-arm64.zip",
				}, {
					"name":                 "cxj-macos-x86_64.zip",
					"browser_download_url": "http://" + r.Host + "/cxj-macos-x86_64.zip",
				}, {
					"name":                 "cxj-linux-x86_64.zip",
					"browser_download_url": "http://" + r.Host + "/cxj-linux-x86_64.zip",
				}, {
					"name":                 "cxj-linux-arm64.zip",
					"browser_download_url": "http://" + r.Host + "/cxj-linux-arm64.zip",
				}},
			})
		case "/cxj-macos-arm64.zip", "/cxj-macos-x86_64.zip", "/cxj-linux-x86_64.zip", "/cxj-linux-arm64.zip":
			downloaded = true
			writeZip(t, w, "cxj", []byte("new-binary"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := &Client{HTTP: srv.Client(), API: srv.URL, Repo: "umuterturk/cxj"}
	got, err := c.Check()
	if err != nil {
		t.Fatal(err)
	}
	if got.UpToDate || got.Latest != "v0.3.0" || got.Current != "v0.2.0" {
		t.Fatalf("%+v", got)
	}
	if downloaded {
		t.Fatal("check downloaded the zip")
	}

	dest := filepath.Join(t.TempDir(), "cxj")
	if err := os.WriteFile(dest, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	applied, err := c.Apply(dest)
	if err != nil {
		t.Fatal(err)
	}
	if applied.UpToDate || applied.Latest != "v0.3.0" {
		t.Fatalf("%+v", applied)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new-binary" {
		t.Fatalf("dest=%q", data)
	}
}

func TestApply_alreadyLatest(t *testing.T) {
	old := version.Version
	t.Cleanup(func() { version.Version = old })
	version.Version = "v0.3.0"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v0.3.0",
			"assets": []map[string]string{{
				"name":                 "cxj-macos-arm64.zip",
				"browser_download_url": "http://example.invalid/x.zip",
			}, {
				"name":                 "cxj-macos-x86_64.zip",
				"browser_download_url": "http://example.invalid/x.zip",
			}, {
				"name":                 "cxj-linux-x86_64.zip",
				"browser_download_url": "http://example.invalid/x.zip",
			}, {
				"name":                 "cxj-linux-arm64.zip",
				"browser_download_url": "http://example.invalid/x.zip",
			}},
		})
	}))
	t.Cleanup(srv.Close)

	c := &Client{HTTP: srv.Client(), API: srv.URL, Repo: "acme/cxj"}
	got, err := c.Apply(filepath.Join(t.TempDir(), "cxj"))
	if err != nil {
		t.Fatal(err)
	}
	if !got.UpToDate {
		t.Fatalf("%+v", got)
	}
}

func writeZip(t *testing.T, w http.ResponseWriter, name string, body []byte) {
	t.Helper()
	zw := zip.NewWriter(w)
	fw, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}
