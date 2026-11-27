// Package upgrade replaces the installed revunk binary with the latest GitHub release.
package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Updater downloads releases from GitHub. APIURL and DownloadURL are overridden in tests.
type Updater struct {
	HTTP        *http.Client
	APIURL      string
	DownloadURL string
}

// Run replaces exe with the latest release when its tag differs from version.
// Binaries under /usr/bin are managed by apt, so Run downloads the matching .deb instead.
func (u Updater) Run(ctx context.Context, exe, version string, out io.Writer) error {
	if version == "dev" {
		return errors.New("development builds cannot be upgraded, install a release instead")
	}
	tag, err := u.latestTag(ctx)
	if err != nil {
		return err
	}
	latest := strings.TrimPrefix(tag, "v")
	if latest == version {
		fmt.Fprintf(out, "revunk %s is up to date\n", version)
		return nil
	}
	sums, err := u.get(ctx, u.releaseURL(tag, "checksums.txt"))
	if err != nil {
		return err
	}
	if filepath.Dir(exe) == "/usr/bin" {
		return u.debPackage(ctx, exe, tag, latest, sums, out)
	}
	name := fmt.Sprintf("revunk_%s_%s_%s.tar.gz", latest, runtime.GOOS, runtime.GOARCH)
	archive, err := u.verified(ctx, u.releaseURL(tag, name), name, sums)
	if err != nil {
		return err
	}
	bin, err := binary(archive)
	if err != nil {
		return err
	}
	if err := replace(exe, bin); err != nil {
		return err
	}
	fmt.Fprintf(out, "upgraded revunk from %s to %s\n", version, latest)
	return nil
}

func (u Updater) debPackage(ctx context.Context, exe, tag, latest string, sums []byte, out io.Writer) error {
	name := fmt.Sprintf("revunk_%s_linux_%s.deb", latest, runtime.GOARCH)
	deb, err := u.verified(ctx, u.releaseURL(tag, name), name, sums)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "revunk-upgrade-")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, deb, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s is installed from a Debian package, so it cannot be replaced.\nDownloaded and verified revunk %s.\nUpgrade with: sudo apt install %s\n", exe, latest, path)
	return nil
}

func (u Updater) latestTag(ctx context.Context) (string, error) {
	data, err := u.get(ctx, u.APIURL+"/releases/latest")
	if err != nil {
		return "", err
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(data, &release); err != nil {
		return "", fmt.Errorf("read latest release: %w", err)
	}
	if release.TagName == "" {
		return "", errors.New("latest release has no tag")
	}
	return release.TagName, nil
}

func (u Updater) releaseURL(tag, name string) string {
	return u.DownloadURL + "/releases/download/" + tag + "/" + name
}

func (u Updater) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := u.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func (u Updater) verified(ctx context.Context, url, name string, sums []byte) ([]byte, error) {
	want, err := checksum(sums, name)
	if err != nil {
		return nil, err
	}
	data, err := u.get(ctx, url)
	if err != nil {
		return nil, err
	}
	got := sha256.Sum256(data)
	if hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("checksum mismatch for %s", name)
	}
	return data, nil
}

// checksum returns the SHA-256 listed for name in a checksums.txt file.
func checksum(sums []byte, name string) (string, error) {
	for line := range strings.Lines(string(sums)) {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no checksum for %s", name)
}

// binary returns the revunk executable stored in a release archive.
func binary(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("archive has no revunk binary")
		}
		if err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == "revunk" {
			return io.ReadAll(tr)
		}
	}
}

// replace writes bin to a temporary file beside exe and renames it over exe, so the swap is atomic.
func replace(exe string, bin []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".revunk-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temporary file beside %s: %w", exe, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), exe)
}
