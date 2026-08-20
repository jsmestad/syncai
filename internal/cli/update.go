package cli

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

const (
	latestReleaseAPI = "https://api.github.com/repos/jsmestad/syncai/releases/latest"
	maxMetadataBytes = 2 << 20
	maxArchiveBytes  = 100 << 20
	maxBinaryBytes   = 100 << 20
	releasePublicKey = "Fr3cEVwgISWT9LzmfGc/m7BWJOM07GE0wA8jDWwsTOg="
)

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type releaseMetadata struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

func updateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Download, verify, and install the latest SyncAI release in place",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			executable, err := os.Executable()
			if err != nil {
				return fmt.Errorf("locating the running executable: %w", err)
			}
			return runUpdate(cmd.Context(), cmd.OutOrStdout(), &http.Client{Timeout: 2 * time.Minute}, latestReleaseAPI, buildVersion(), executable, runtime.GOOS, runtime.GOARCH, trustedReleaseKey())
		},
	}
}

func runUpdate(ctx context.Context, out io.Writer, client *http.Client, releaseURL, currentVersion, executable, goos, goarch string, trustedKey ed25519.PublicKey) error {
	if currentVersion == "dev" {
		return fmt.Errorf("development builds cannot update in place; install a released SyncAI binary first")
	}
	if goos != "darwin" && goos != "linux" {
		return fmt.Errorf("self-update is not supported on %s", goos)
	}
	resolvedExecutable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return fmt.Errorf("resolving executable path %s: %w", executable, err)
	}

	release, err := fetchRelease(ctx, client, releaseURL)
	if err != nil {
		return err
	}
	latestVersion := strings.TrimPrefix(release.TagName, "v")
	if latestVersion == "" {
		return fmt.Errorf("latest release has an empty tag")
	}
	if latestVersion == strings.TrimPrefix(currentVersion, "v") {
		if _, err := fmt.Fprintf(out, "SyncAI %s is already current.\n", latestVersion); err != nil {
			return fmt.Errorf("writing current-version confirmation: %w", err)
		}
		return nil
	}
	currentSemanticVersion := "v" + strings.TrimPrefix(currentVersion, "v")
	latestSemanticVersion := "v" + latestVersion
	if !semver.IsValid(currentSemanticVersion) || !semver.IsValid(latestSemanticVersion) {
		return fmt.Errorf("cannot compare current version %q with latest release %q", currentVersion, release.TagName)
	}
	if semver.Compare(latestSemanticVersion, currentSemanticVersion) <= 0 {
		return fmt.Errorf("refusing to downgrade SyncAI from %s to %s", strings.TrimPrefix(currentVersion, "v"), latestVersion)
	}

	archiveName := fmt.Sprintf("syncai_%s_%s.tar.gz", goos, goarch)
	archiveAsset, ok := findAsset(release.Assets, archiveName)
	if !ok {
		return fmt.Errorf("release %s does not contain %s", release.TagName, archiveName)
	}
	checksumAsset, ok := findAsset(release.Assets, "checksums.txt")
	if !ok {
		return fmt.Errorf("release %s does not contain checksums.txt", release.TagName)
	}
	signatureAsset, ok := findAsset(release.Assets, "checksums.txt.ed25519")
	if !ok {
		return fmt.Errorf("release %s does not contain checksums.txt.ed25519", release.TagName)
	}

	checksums, err := downloadBytes(ctx, client, checksumAsset.URL, maxMetadataBytes)
	if err != nil {
		return fmt.Errorf("downloading checksums: %w", err)
	}
	signature, err := downloadBytes(ctx, client, signatureAsset.URL, ed25519.SignatureSize)
	if err != nil {
		return fmt.Errorf("downloading checksum signature: %w", err)
	}
	signedChecksums := append([]byte(release.TagName+"\n"), checksums...)
	if len(signature) != ed25519.SignatureSize || !ed25519.Verify(trustedKey, signedChecksums, signature) {
		return fmt.Errorf("release checksum signature is invalid")
	}
	expectedHash, err := checksumFor(checksums, archiveName)
	if err != nil {
		return err
	}
	archivePath, actualHash, err := downloadFile(ctx, client, archiveAsset.URL, maxArchiveBytes)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", archiveName, err)
	}
	defer os.Remove(archivePath)
	if actualHash != expectedHash {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", archiveName, expectedHash, actualHash)
	}
	if err := replaceFromArchive(archivePath, resolvedExecutable); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Updated SyncAI from %s to %s at %s.\n", strings.TrimPrefix(currentVersion, "v"), latestVersion, resolvedExecutable); err != nil {
		return fmt.Errorf("updated SyncAI to %s at %s, but writing confirmation failed: %w", latestVersion, resolvedExecutable, err)
	}
	return nil
}

func trustedReleaseKey() ed25519.PublicKey {
	key, err := base64.StdEncoding.DecodeString(releasePublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		panic("invalid embedded SyncAI release public key")
	}
	return ed25519.PublicKey(key)
}

func fetchRelease(ctx context.Context, client *http.Client, url string) (releaseMetadata, error) {
	raw, err := downloadBytes(ctx, client, url, maxMetadataBytes)
	if err != nil {
		return releaseMetadata{}, fmt.Errorf("fetching latest release: %w", err)
	}
	var release releaseMetadata
	if err := json.Unmarshal(raw, &release); err != nil {
		return releaseMetadata{}, fmt.Errorf("decoding latest release: %w", err)
	}
	return release, nil
}

func findAsset(assets []releaseAsset, name string) (releaseAsset, bool) {
	for _, asset := range assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return releaseAsset{}, false
}

func downloadBytes(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	response, err := request(ctx, client, url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return raw, nil
}

func downloadFile(ctx context.Context, client *http.Client, url string, limit int64) (string, string, error) {
	response, err := request(ctx, client, url)
	if err != nil {
		return "", "", err
	}
	defer response.Body.Close()
	temporary, err := os.CreateTemp("", "syncai-update-*.tar.gz")
	if err != nil {
		return "", "", err
	}
	path := temporary.Name()
	keep := false
	defer func() {
		_ = temporary.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(response.Body, limit+1))
	if err != nil {
		return "", "", err
	}
	if written > limit {
		return "", "", fmt.Errorf("response exceeds %d bytes", limit)
	}
	if err := temporary.Close(); err != nil {
		return "", "", err
	}
	keep = true
	return path, hex.EncodeToString(hash.Sum(nil)), nil
}

func request(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if request.URL.Scheme != "https" {
		return nil, fmt.Errorf("refusing non-HTTPS update URL %s", url)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "syncai-update")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.Request != nil && response.Request.URL.Scheme != "https" {
		_ = response.Body.Close()
		return nil, fmt.Errorf("refusing update redirect to non-HTTPS URL %s", response.Request.URL)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_ = response.Body.Close()
		return nil, fmt.Errorf("%s returned %s", url, response.Status)
	}
	return response, nil
}

func checksumFor(manifest []byte, filename string) (string, error) {
	for _, line := range strings.Split(string(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == filename {
			if len(fields[0]) != sha256.Size*2 {
				return "", fmt.Errorf("invalid checksum for %s", filename)
			}
			if _, err := hex.DecodeString(fields[0]); err != nil {
				return "", fmt.Errorf("invalid checksum for %s: %w", filename, err)
			}
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("checksums.txt does not contain %s", filename)
}

func replaceFromArchive(archivePath, executable string) error {
	archive, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer archive.Close()
	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return fmt.Errorf("opening release archive: %w", err)
	}
	defer gzipReader.Close()

	info, err := os.Stat(executable)
	if err != nil {
		return fmt.Errorf("inspecting current executable: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(executable), ".syncai-update-*")
	if err != nil {
		return fmt.Errorf("creating replacement beside %s: %w", executable, err)
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		_ = os.Remove(temporaryPath)
	}()

	found := false
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading release archive: %w", err)
		}
		if header.Name != "syncai" {
			continue
		}
		if found || header.Typeflag != tar.TypeReg || header.Size < 0 || header.Size > maxBinaryBytes {
			return fmt.Errorf("release archive contains an invalid syncai executable")
		}
		written, err := io.CopyN(temporary, tarReader, header.Size)
		if err != nil || written != header.Size {
			return fmt.Errorf("extracting syncai executable: %w", err)
		}
		found = true
	}
	if !found {
		return fmt.Errorf("release archive does not contain syncai")
	}
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		return fmt.Errorf("preserving executable permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("syncing replacement executable: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("closing replacement executable: %w", err)
	}
	closed = true
	if err := os.Rename(temporaryPath, executable); err != nil {
		return fmt.Errorf("replacing executable %s: %w", executable, err)
	}
	return nil
}
