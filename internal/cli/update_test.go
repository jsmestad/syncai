package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type updateRoundTripFunc func(*http.Request) (*http.Response, error)

type failingUpdateWriter struct{}

func (failingUpdateWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("output unavailable")
}

func (function updateRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestRunUpdateVerifiesAndReplacesExecutable(t *testing.T) {
	archive := updateArchive(t, []byte("new binary"))
	hash := sha256.Sum256(archive)
	publicKey, privateKey := updateSigningKey()
	checksums := fmt.Appendf(nil, "%s  syncai_darwin_arm64.tar.gz\n", hex.EncodeToString(hash[:]))
	client := updateClient(map[string][]byte{
		"/latest":    []byte(`{"tag_name":"v1.1.0","assets":[{"name":"syncai_darwin_arm64.tar.gz","browser_download_url":"https://example.test/archive"},{"name":"checksums.txt","browser_download_url":"https://example.test/checksums"},{"name":"checksums.txt.ed25519","browser_download_url":"https://example.test/signature"}]}`),
		"/archive":   archive,
		"/checksums": checksums,
		"/signature": ed25519.Sign(privateKey, append([]byte("v1.1.0\n"), checksums...)),
	})

	executable := filepath.Join(t.TempDir(), "syncai")
	if err := os.WriteFile(executable, []byte("old binary"), 0o751); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := runUpdate(context.Background(), &output, client, "https://example.test/latest", "1.0.0", executable, "darwin", "arm64", publicKey); err != nil {
		t.Fatalf("runUpdate: %v", err)
	}
	updated, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if string(updated) != "new binary" {
		t.Fatalf("executable = %q", updated)
	}
	info, err := os.Stat(executable)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o751 {
		t.Fatalf("permissions = %o", info.Mode().Perm())
	}
	if !strings.Contains(output.String(), "Updated SyncAI from 1.0.0 to 1.1.0") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRunUpdateLeavesExecutableOnChecksumMismatch(t *testing.T) {
	archive := updateArchive(t, []byte("new binary"))
	publicKey, privateKey := updateSigningKey()
	checksums := fmt.Appendf(nil, "%064d  syncai_linux_amd64.tar.gz\n", 0)
	client := updateClient(map[string][]byte{
		"/latest":    []byte(`{"tag_name":"v1.1.0","assets":[{"name":"syncai_linux_amd64.tar.gz","browser_download_url":"https://example.test/archive"},{"name":"checksums.txt","browser_download_url":"https://example.test/checksums"},{"name":"checksums.txt.ed25519","browser_download_url":"https://example.test/signature"}]}`),
		"/archive":   archive,
		"/checksums": checksums,
		"/signature": ed25519.Sign(privateKey, append([]byte("v1.1.0\n"), checksums...)),
	})

	executable := filepath.Join(t.TempDir(), "syncai")
	if err := os.WriteFile(executable, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := runUpdate(context.Background(), &bytes.Buffer{}, client, "https://example.test/latest", "1.0.0", executable, "linux", "amd64", publicKey)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("runUpdate error = %v", err)
	}
	current, readErr := os.ReadFile(executable)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(current) != "old binary" {
		t.Fatalf("executable changed after failed verification: %q", current)
	}
}

func TestRunUpdateReportsConfirmationFailureAfterReplacement(t *testing.T) {
	archive := updateArchive(t, []byte("new binary"))
	hash := sha256.Sum256(archive)
	publicKey, privateKey := updateSigningKey()
	checksums := fmt.Appendf(nil, "%s  syncai_linux_arm64.tar.gz\n", hex.EncodeToString(hash[:]))
	client := updateClient(map[string][]byte{
		"/latest":    []byte(`{"tag_name":"v1.1.0","assets":[{"name":"syncai_linux_arm64.tar.gz","browser_download_url":"https://example.test/archive"},{"name":"checksums.txt","browser_download_url":"https://example.test/checksums"},{"name":"checksums.txt.ed25519","browser_download_url":"https://example.test/signature"}]}`),
		"/archive":   archive,
		"/checksums": checksums,
		"/signature": ed25519.Sign(privateKey, append([]byte("v1.1.0\n"), checksums...)),
	})
	executable := filepath.Join(t.TempDir(), "syncai")
	if err := os.WriteFile(executable, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := runUpdate(context.Background(), failingUpdateWriter{}, client, "https://example.test/latest", "1.0.0", executable, "linux", "arm64", publicKey)
	if err == nil || !strings.Contains(err.Error(), "updated SyncAI to 1.1.0") || !strings.Contains(err.Error(), "output unavailable") {
		t.Fatalf("runUpdate error = %v", err)
	}
	updated, readErr := os.ReadFile(executable)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(updated) != "new binary" {
		t.Fatalf("executable = %q", updated)
	}
}

func TestRunUpdateReportsCurrentVersionConfirmationFailure(t *testing.T) {
	publicKey, _ := updateSigningKey()
	client := updateClient(map[string][]byte{"/latest": []byte(`{"tag_name":"v1.0.0","assets":[]}`)})
	executable := filepath.Join(t.TempDir(), "syncai")
	if err := os.WriteFile(executable, []byte("current binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := runUpdate(context.Background(), failingUpdateWriter{}, client, "https://example.test/latest", "1.0.0", executable, "darwin", "arm64", publicKey)
	if err == nil || !strings.Contains(err.Error(), "writing current-version confirmation") || !strings.Contains(err.Error(), "output unavailable") {
		t.Fatalf("runUpdate error = %v", err)
	}
}

func updateClient(responses map[string][]byte) *http.Client {
	return &http.Client{Transport: updateRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, ok := responses[request.URL.Path]
		if !ok {
			return &http.Response{StatusCode: http.StatusNotFound, Status: "404 Not Found", Body: io.NopCloser(strings.NewReader("not found"))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})}
}

func TestRunUpdateRejectsDevelopmentBuild(t *testing.T) {
	err := runUpdate(context.Background(), &bytes.Buffer{}, http.DefaultClient, "https://example.invalid", "dev", "/tmp/syncai", "darwin", "arm64", trustedReleaseKey())
	if err == nil || !strings.Contains(err.Error(), "development builds cannot update") {
		t.Fatalf("runUpdate error = %v", err)
	}
}

func TestRunUpdateRefusesOlderRelease(t *testing.T) {
	publicKey, _ := updateSigningKey()
	client := updateClient(map[string][]byte{"/latest": []byte(`{"tag_name":"v1.9.0","assets":[]}`)})
	executable := filepath.Join(t.TempDir(), "syncai")
	if err := os.WriteFile(executable, []byte("current"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := runUpdate(context.Background(), &bytes.Buffer{}, client, "https://example.test/latest", "2.0.0", executable, "darwin", "arm64", publicKey)
	if err == nil || !strings.Contains(err.Error(), "refusing to downgrade") {
		t.Fatalf("runUpdate error = %v", err)
	}
}

func TestRequestRejectsNonHTTPSURL(t *testing.T) {
	_, err := request(context.Background(), http.DefaultClient, "http://example.test/release")
	if err == nil || !strings.Contains(err.Error(), "refusing non-HTTPS") {
		t.Fatalf("request error = %v", err)
	}
}

func updateArchive(t *testing.T, binary []byte) []byte {
	t.Helper()
	var compressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressed)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "syncai", Mode: 0o755, Size: int64(len(binary)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return compressed.Bytes()
}

func updateSigningKey() (ed25519.PublicKey, ed25519.PrivateKey) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize))
	return privateKey.Public().(ed25519.PublicKey), privateKey
}
