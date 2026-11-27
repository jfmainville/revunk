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

type Updater struct {
	HTTP        *http.Client
	APIURL      string
	DownloadURL string
}

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
		_, err = fmt.Fprintf(out, "revunk %s is up to date\n", version)
		return err
	}
	sums, err := u.get(ctx, u.releaseURL(tag, "checksums.txt"))
	if err != nil {
		return err
	}
	if filepath.Dir(exe) == "/usr/bin" {
		return u.debPackage(ctx, exe, tag, sums, out)
	}
	name := fmt.Sprintf("revunk_%s_%s_%s.tar.gz", latest, runtime.GOOS, runtime.GOARCH)
	archive, err := u.verified(ctx, tag, name, sums)
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
	_, err = fmt.Fprintf(out, "upgraded revunk from %s to %s\n", version, latest)
	return err
}

func (u Updater) debPackage(ctx context.Context, exe, tag string, sums []byte, out io.Writer) error {
	latest := strings.TrimPrefix(tag, "v")
	name := fmt.Sprintf("revunk_%s_linux_%s.deb", latest, runtime.GOARCH)
	deb, err := u.verified(ctx, tag, name, sums)
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
	_, err = fmt.Fprintf(out, "%s is installed from a Debian package, so it cannot be replaced.\nDownloaded and verified revunk %s.\nUpgrade with: sudo apt install %s\n", exe, latest, path)
	return err
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
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Join(fmt.Errorf("get %s: %s", url, resp.Status), resp.Body.Close())
	}
	body, err := io.ReadAll(resp.Body)
	return body, errors.Join(err, resp.Body.Close())
}

func (u Updater) verified(ctx context.Context, tag, name string, sums []byte) ([]byte, error) {
	want, err := checksum(sums, name)
	if err != nil {
		return nil, err
	}
	data, err := u.get(ctx, u.releaseURL(tag, name))
	if err != nil {
		return nil, err
	}
	got := sha256.Sum256(data)
	if hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("checksum mismatch for %s", name)
	}
	return data, nil
}

func checksum(sums []byte, name string) (string, error) {
	for line := range strings.Lines(string(sums)) {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no checksum for %s", name)
}

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

func replace(exe string, bin []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".revunk-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temporary file beside %s: %w", exe, err)
	}
	_, err = tmp.Write(bin)
	err = errors.Join(err, tmp.Close())
	if err == nil {
		err = os.Chmod(tmp.Name(), 0o755)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), exe)
	}
	if err != nil {
		return errors.Join(fmt.Errorf("replace %s: %w", exe, err), os.Remove(tmp.Name()))
	}
	return nil
}
