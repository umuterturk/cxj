package needle

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrDeclined is returned when the user says no to the Needle download prompt.
var ErrDeclined = errors.New("ask skipped: Needle 3 not downloaded")

const (
	engineURL  = "https://huggingface.co/Cactus-Compute/needle3/resolve/main/python/cactus_needle-%s-py3-none-%s.whl"
	weightsURL = "https://huggingface.co/Cactus-Compute/needle3/resolve/main/" + weightsName
)

// Ready reports whether libneedle and needle3.cact are already on disk.
func Ready() bool {
	lib, err := FindLibrary()
	if err != nil {
		return false
	}
	return weightsBeside(lib) != ""
}

// InstallDir is where a prompted download writes Needle 3 files.
func InstallDir() string {
	if d := os.Getenv("CX_NEEDLE_DIR"); d != "" {
		return d
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			dir = filepath.Dir(real)
		}
		if !underTemp(dir) && writable(dir) {
			return dir
		}
	}
	if wd, err := os.Getwd(); err == nil {
		return filepath.Join(wd, "needle")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("needle")
	}
	return filepath.Join(home, ".cache", "cactus-needle", "v3", needle3Ver)
}

// Ensure makes Needle 3 available, prompting to download when files are missing.
func Ensure(in io.Reader, out io.Writer, auto bool) error {
	if Ready() {
		return nil
	}
	dest := InstallDir()
	if !auto {
		if !isTTY(os.Stdin) || !isTTY(os.Stderr) {
			return fmt.Errorf("ask needs Needle 3 (~35 MB) in %s\nRe-run `cxj ask` in a terminal to download, or `cxj ask --download`", dest)
		}
		fmt.Fprintf(out, "ask needs Needle 3 (~35 MB): libneedle + %s\nDownload to %s? [y/N] ", weightsName, dest)
		var reply string
		if _, err := fmt.Fscanln(in, &reply); err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if !isYes(reply) {
			return ErrDeclined
		}
	}
	if err := Download(dest, out); err != nil {
		return err
	}
	if !Ready() {
		return fmt.Errorf("Needle 3 downloaded to %s but was not found on the search path", dest)
	}
	return nil
}

// Download fetches the Needle 3 engine and weights into dest.
func Download(dest string, out io.Writer) error {
	libName, tag, members, err := platformAssets()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	libPath := filepath.Join(dest, libName)
	if _, err := os.Stat(libPath); err != nil {
		fmt.Fprintf(out, "downloading Needle 3 engine (%s)\n", tag)
		if err := fetchLib(fmt.Sprintf(engineURL, needle3Ver, tag), members, libPath); err != nil {
			return err
		}
	}
	wPath := filepath.Join(dest, weightsName)
	if _, err := os.Stat(wPath); err != nil {
		fmt.Fprintf(out, "downloading %s\n", weightsName)
		if err := fetchFile(weightsURL, wPath); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "Needle 3 installed in %s\n", dest)
	return nil
}

func platformAssets() (libName, tag string, members []string, err error) {
	switch runtime.GOOS + "-" + runtime.GOARCH {
	case "darwin-arm64":
		return libDarwin, "macosx_11_0_arm64", []string{"needle/" + libDarwin3, "needle/" + libDarwin}, nil
	case "darwin-amd64":
		return libDarwin, "macosx_11_0_x86_64", []string{"needle/" + libDarwin3, "needle/" + libDarwin}, nil
	case "linux-amd64":
		return libUnix, "manylinux2014_x86_64", []string{"needle/" + libUnix3, "needle/" + libUnix}, nil
	case "linux-arm64":
		return libUnix, "manylinux2014_aarch64", []string{"needle/" + libUnix3, "needle/" + libUnix}, nil
	default:
		return "", "", nil, fmt.Errorf("Needle 3 is not published for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func fetchLib(url string, members []string, dest string) error {
	tmp, err := os.CreateTemp("", "cxj-needle-*.whl")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := copyURL(url, tmp); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	zr, err := zip.OpenReader(tmpName)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, name := range members {
		for i := range zr.File {
			if zr.File[i].Name != name {
				continue
			}
			rc, err := zr.File[i].Open()
			if err != nil {
				return err
			}
			out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
			if err != nil {
				rc.Close()
				return err
			}
			_, err = io.Copy(out, rc)
			rc.Close()
			cerr := out.Close()
			if err != nil {
				return err
			}
			return cerr
		}
	}
	return fmt.Errorf("wheel missing %v", members)
}

func fetchFile(url, dest string) error {
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	err = copyURL(url, f)
	cerr := f.Close()
	if err != nil {
		os.Remove(dest)
		return err
	}
	return cerr
}

func copyURL(url string, dest io.Writer) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "cxj")
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}
	_, err = io.Copy(dest, resp.Body)
	return err
}

func isYes(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

func isTTY(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".cxj-write-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

func underTemp(dir string) bool {
	tmp := os.TempDir()
	rel, err := filepath.Rel(tmp, dir)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}
