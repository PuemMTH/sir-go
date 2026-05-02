package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
)

const upgradeRepo = "PuemMTH/sir-go"

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func Run(version string) error {
	fmt.Printf("  Current version: %s\n", version)
	fmt.Println("  Checking for updates...")

	release, err := fetchLatestRelease(version)
	if err != nil {
		return fmt.Errorf("could not fetch release info: %w", err)
	}

	if release.TagName == version {
		fmt.Printf("  Already up to date (%s)\n", version)
		return nil
	}

	fmt.Printf("  New version available: %s\n", release.TagName)

	assetName := fmt.Sprintf("sir_%s_%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}

	var downloadURL string
	for _, a := range release.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("no binary found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	expectedSum, err := fetchChecksum(release, assetName)
	if err != nil {
		fmt.Printf("  Warning: could not verify checksum: %v\n", err)
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}

	fmt.Printf("  Downloading %s...\n", assetName)
	data, err := httpGet(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	if expectedSum != "" {
		sum := sha256.Sum256(data)
		got := hex.EncodeToString(sum[:])
		if got != expectedSum {
			return fmt.Errorf("checksum mismatch: got %s, want %s", got, expectedSum)
		}
		fmt.Println("  Checksum verified.")
	}

	if err := replaceSelf(execPath, data); err != nil {
		return fmt.Errorf("could not replace binary: %w", err)
	}

	fmt.Printf("  ✓ Upgraded to %s\n", release.TagName)
	return nil
}

func fetchLatestRelease(version string) (*ghRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", upgradeRepo)
	data, err := httpGetWithAccept(url, "application/vnd.github+json", version)
	if err != nil {
		return nil, err
	}
	var rel ghRelease
	if err := json.Unmarshal(data, &rel); err != nil {
		return nil, err
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("empty release tag — check that %s has published releases", upgradeRepo)
	}
	return &rel, nil
}

func fetchChecksum(release *ghRelease, assetName string) (string, error) {
	var checksumURL string
	for _, a := range release.Assets {
		if a.Name == "checksums.txt" {
			checksumURL = a.BrowserDownloadURL
			break
		}
	}
	if checksumURL == "" {
		return "", fmt.Errorf("checksums.txt not found in release")
	}
	data, err := httpGet(checksumURL)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == assetName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no checksum entry for %s", assetName)
}

func httpGet(url string) ([]byte, error) {
	return httpGetWithAccept(url, "application/octet-stream", "")
}

func httpGetWithAccept(url, accept, version string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sir-upgrade/"+version)
	req.Header.Set("Accept", accept)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

func replaceSelf(execPath string, data []byte) error {
	tmp, err := os.CreateTemp("", "sir-upgrade-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0755); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	return os.Rename(tmpPath, execPath)
}
