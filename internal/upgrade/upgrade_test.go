package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func tarball(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func archiveName() string {
	return fmt.Sprintf("revunk_1.2.0_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
}

// release returns the files of a GitHub release tagged v1.2.0, keyed by request path.
func release(t *testing.T, archive []byte) map[string][]byte {
	t.Helper()
	deb := []byte("debian package")
	files := map[string][]byte{
		"/api/releases/latest": []byte(`{"tag_name":"v1.2.0"}`),
	}
	var sums strings.Builder
	for name, body := range map[string][]byte{
		archiveName(): archive,
		fmt.Sprintf("revunk_1.2.0_linux_%s.deb", runtime.GOARCH): deb,
	} {
		sum := sha256.Sum256(body)
		fmt.Fprintf(&sums, "%x  %s\n", sum, name)
		files["/releases/download/v1.2.0/"+name] = body
	}
	files["/releases/download/v1.2.0/checksums.txt"] = []byte(sums.String())
	return files
}

func server(t *testing.T, files map[string][]byte) Updater {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return Updater{HTTP: srv.Client(), APIURL: srv.URL + "/api", DownloadURL: srv.URL}
}

func TestRunReplacesBinaryWithVerifiedRelease(t *testing.T) {
	files := release(t, tarball(t, "revunk", []byte("new binary")))
	exe := filepath.Join(t.TempDir(), "revunk")
	if err := os.WriteFile(exe, []byte("old binary"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder

	if err := server(t, files).Run(context.Background(), exe, "1.1.0", &out); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(exe)
	if err != nil || string(got) != "new binary" {
		t.Fatalf("binary = %q, %v, want the release binary", got, err)
	}
	info, err := os.Stat(exe)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, %v, want 0755", info.Mode().Perm(), err)
	}
	entries, _ := os.ReadDir(filepath.Dir(exe))
	if len(entries) != 1 {
		t.Fatalf("directory has %d entries, want only the binary", len(entries))
	}
	if !strings.Contains(out.String(), "upgraded revunk from 1.1.0 to 1.2.0") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunReportsUpToDateWithoutDownloading(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "revunk")
	if err := os.WriteFile(exe, []byte("current"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"/api/releases/latest": []byte(`{"tag_name":"v1.2.0"}`)}
	var out strings.Builder

	if err := server(t, files).Run(context.Background(), exe, "1.2.0", &out); err != nil {
		t.Fatal(err)
	}

	if got, _ := os.ReadFile(exe); string(got) != "current" {
		t.Fatalf("binary = %q, want it unchanged", got)
	}
	if !strings.Contains(out.String(), "up to date") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunRefusesDevelopmentBuilds(t *testing.T) {
	var out strings.Builder
	err := server(t, nil).Run(context.Background(), "/home/me/bin/revunk", "dev", &out)
	if err == nil || !strings.Contains(err.Error(), "development builds") {
		t.Fatalf("error = %v, want a development build refusal", err)
	}
}

func TestRunFailsWithoutReplacingBinary(t *testing.T) {
	good := tarball(t, "revunk", []byte("new binary"))
	cases := map[string]struct {
		files map[string][]byte
		want  string
	}{
		"checksum mismatch": {
			files: func() map[string][]byte {
				f := release(t, good)
				f["/releases/download/v1.2.0/checksums.txt"] = []byte(strings.Repeat("0", 64) + "  " + archiveName() + "\n")
				return f
			}(),
			want: "checksum mismatch",
		},
		"no checksum listed": {
			files: func() map[string][]byte {
				f := release(t, good)
				f["/releases/download/v1.2.0/checksums.txt"] = []byte("")
				return f
			}(),
			want: "no checksum for",
		},
		"archive without binary": {
			files: release(t, tarball(t, "README.md", []byte("readme"))),
			want:  "archive has no revunk binary",
		},
		"latest release without tag": {
			files: map[string][]byte{"/api/releases/latest": []byte(`{}`)},
			want:  "latest release has no tag",
		},
		"invalid latest release": {
			files: map[string][]byte{"/api/releases/latest": []byte(`not json`)},
			want:  "read latest release",
		},
		"release lookup fails": {
			files: map[string][]byte{},
			want:  "404",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			exe := filepath.Join(t.TempDir(), "revunk")
			if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
				t.Fatal(err)
			}
			var out strings.Builder

			err := server(t, tc.files).Run(context.Background(), exe, "1.1.0", &out)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.want)
			}
			if got, _ := os.ReadFile(exe); string(got) != "old binary" {
				t.Fatalf("binary = %q, want it unchanged", got)
			}
		})
	}
}

func TestRunDownloadsDebianPackageForUsrBinInstalls(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	var out strings.Builder

	err := server(t, release(t, tarball(t, "revunk", []byte("unused")))).Run(context.Background(), "/usr/bin/revunk", "1.1.0", &out)

	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "sudo apt install") {
		t.Fatalf("output = %q, want apt guidance", out.String())
	}
	paths, _ := filepath.Glob(filepath.Join(os.Getenv("TMPDIR"), "revunk-upgrade-*", "revunk_1.2.0_linux_*.deb"))
	if len(paths) != 1 {
		t.Fatalf("downloaded packages = %q, want one .deb", paths)
	}
	if got, err := os.ReadFile(paths[0]); err != nil || string(got) != "debian package" {
		t.Fatalf("package = %q, %v, want the verified .deb", got, err)
	}
}

func TestChecksumFindsNamedFile(t *testing.T) {
	sums := []byte("aaa  revunk_1.2.0_linux_amd64.tar.gz\nbbb  revunk_1.2.0_linux_arm64.tar.gz\n")

	got, err := checksum(sums, "revunk_1.2.0_linux_arm64.tar.gz")
	if err != nil || got != "bbb" {
		t.Fatalf("checksum = %q, %v, want bbb", got, err)
	}
	if _, err := checksum(sums, "revunk_1.2.0_darwin_amd64.tar.gz"); err == nil {
		t.Fatal("checksum of an unlisted file should fail")
	}
}

func TestBinaryReturnsRevunkEntry(t *testing.T) {
	archive := tarball(t, "revunk", []byte("the binary"))
	got, err := binary(archive)
	if err != nil || string(got) != "the binary" {
		t.Fatalf("binary = %q, %v", got, err)
	}
	if _, err := binary([]byte("not gzip")); err == nil {
		t.Fatal("binary should reject data that is not a gzip archive")
	}
}
