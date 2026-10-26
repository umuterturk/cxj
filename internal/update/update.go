// Package update fetches and installs the latest GitHub release of cxj.
package update

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"example.com/cx/internal/version"
)

const (
	defaultRepo = "umuterturk/cxj"
	defaultAPI  = "https://api.github.com"
)

// Client talks to the GitHub Releases API.
type Client struct {
	HTTP *http.Client
	Repo string
	API  string
}

func (c *Client) http() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (c *Client) repo() string {
	if c != nil && c.Repo != "" {
		return c.Repo
	}
	if r := strings.TrimSpace(os.Getenv("CXJ_REPO")); r != "" {
		return r
	}
	return defaultRepo
}

func (c *Client) api() string {
	if c != nil && c.API != "" {
		return strings.TrimRight(c.API, "/")
	}
	if a := strings.TrimSpace(os.Getenv("CXJ_RELEASE_API")); a != "" {
		return strings.TrimRight(a, "/")
	}
	return defaultAPI
}

// Release is one GitHub release plus the asset for this platform.
type Release struct {
	Tag      string
	Asset    string
	URL      string
	Current  string
	UpToDate bool
}

// Result is the outcome of Check or Apply.
type Result struct {
	Current  string `json:"current"`
	Latest   string `json:"latest"`
	UpToDate bool   `json:"up_to_date"`
	Asset    string `json:"asset,omitempty"`
	Path     string `json:"path,omitempty"`
}

// AssetName is the published zip for goos/goarch.
func AssetName(goos, goarch string) (string, error) {
	switch goos + "-" + goarch {
	case "darwin-arm64":
		return "cxj-macos-arm64.zip", nil
	case "darwin-amd64":
		return "cxj-macos-x86_64.zip", nil
	case "linux-amd64":
		return "cxj-linux-x86_64.zip", nil
	case "linux-arm64":
		return "cxj-linux-arm64.zip", nil
	default:
		return "", fmt.Errorf("no cxj release asset for %s/%s", goos, goarch)
	}
}

// Latest fetches the newest release and picks this platform's zip.
func (c *Client) Latest() (Release, error) {
	want, err := AssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return Release{}, err
	}
	url := c.api() + "/repos/" + c.repo() + "/releases/latest"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "cxj")
	res, err := c.http().Do(req)
	if err != nil {
		return Release{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return Release{}, err
	}
	if res.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("github releases: %s", res.Status)
	}
	var payload struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Release{}, err
	}
	if payload.TagName == "" {
		return Release{}, fmt.Errorf("github releases: empty tag")
	}
	cur := version.Current()
	out := Release{Tag: payload.TagName, Current: cur, UpToDate: version.Same(cur, payload.TagName)}
	for _, a := range payload.Assets {
		if a.Name == want {
			out.Asset = a.Name
			out.URL = a.URL
			break
		}
	}
	if out.URL == "" {
		return out, fmt.Errorf("release %s has no %s", payload.TagName, want)
	}
	return out, nil
}

// Check compares the running binary to the latest release (no download).
func (c *Client) Check() (Result, error) {
	rel, err := c.Latest()
	if err != nil {
		return Result{}, err
	}
	return Result{
		Current:  rel.Current,
		Latest:   rel.Tag,
		UpToDate: rel.UpToDate,
		Asset:    rel.Asset,
	}, nil
}

// Apply downloads the latest release and replaces dest (the running executable).
func (c *Client) Apply(dest string) (Result, error) {
	rel, err := c.Latest()
	if err != nil {
		return Result{}, err
	}
	out := Result{Current: rel.Current, Latest: rel.Tag, UpToDate: rel.UpToDate, Asset: rel.Asset, Path: dest}
	if rel.UpToDate {
		return out, nil
	}
	tmp, err := os.CreateTemp("", "cxj-update-*.zip")
	if err != nil {
		return out, err
	}
	zipPath := tmp.Name()
	defer os.Remove(zipPath)
	if err := download(c.http(), rel.URL, tmp); err != nil {
		tmp.Close()
		return out, err
	}
	if err := tmp.Close(); err != nil {
		return out, err
	}
	bin, err := extractBinary(zipPath)
	if err != nil {
		return out, err
	}
	defer os.Remove(bin)
	if err := replaceFile(bin, dest); err != nil {
		return out, err
	}
	out.Path = dest
	return out, nil
}

// Executable is the real path of the running cxj binary.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		return real, nil
	}
	return exe, nil
}

func download(client *http.Client, url string, w io.Writer) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "cxj")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, res.Status)
	}
	_, err = io.Copy(w, res.Body)
	return err
}

func extractBinary(zipPath string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		if base != "cxj" && base != "cxj.exe" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		tmp, err := os.CreateTemp("", "cxj-bin-*")
		if err != nil {
			rc.Close()
			return "", err
		}
		if _, err := io.Copy(tmp, rc); err != nil {
			rc.Close()
			tmp.Close()
			os.Remove(tmp.Name())
			return "", err
		}
		rc.Close()
		if err := tmp.Chmod(0755); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return "", err
		}
		if err := tmp.Close(); err != nil {
			os.Remove(tmp.Name())
			return "", err
		}
		return tmp.Name(), nil
	}
	return "", fmt.Errorf("zip has no cxj binary")
}

func replaceFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".cxj-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("replace %s: %w", dest, err)
	}
	return nil
}
