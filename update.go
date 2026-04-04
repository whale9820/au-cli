package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	version   = "v0.3.6-alpha"
	repoOwner = "cfpy67"
	repoName  = "au-cli"
)

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func checkUpdate() (tag string, downloadURL string, err error) {
	client := &http.Client{Timeout: 5 * time.Second}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)
	resp, err := client.Get(apiURL)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", "", err
	}
	return rel.TagName, assetDownloadURL(rel.Assets), nil
}

func assetDownloadURL(assets []ghAsset) string {
	name := platformAssetName()
	for _, a := range assets {
		if a.Name == name {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

func platformAssetName() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("au-%s-%s%s", runtime.GOOS, runtime.GOARCH, ext)
}

func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v = v[:i]
	}
	parts := strings.SplitN(v, ".", 3)
	var nums [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		nums[i], _ = strconv.Atoi(p)
	}
	return nums
}

func isNewer(remote, local string) bool {
	r, l := parseSemver(remote), parseSemver(local)
	for i := range r {
		if r[i] > l[i] {
			return true
		}
		if r[i] < l[i] {
			return false
		}
	}
	return false
}

func selfUpdate(downloadURL string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}

	fmt.Printf("  downloading %s...\n", platformAssetName())
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	// Write to a temp file in the system temp dir (always writable).
	tmp, err := os.CreateTemp("", "au-update-*")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	tmp.Close()
	if err := os.Chmod(tmpName, 0755); err != nil {
		return fmt.Errorf("cannot chmod update: %w", err)
	}

	// Try a direct rename first (works when we own the binary location).
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err == nil {
		if err2 := os.Rename(tmpName, exe); err2 != nil {
			os.Rename(old, exe) // restore
			return fmt.Errorf("cannot install update: %w", err2)
		}
		os.Remove(old)
		return nil
	}

	// Fall back to sudo mv for system-wide installs (e.g. /usr/local/bin).
	fmt.Println("  needs elevated permissions — running sudo mv...")
	cmd := exec.Command("sudo", "mv", tmpName, exe)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo mv failed: %w", err)
	}
	if err := exec.Command("sudo", "chmod", "755", exe).Run(); err != nil {
		return fmt.Errorf("sudo chmod failed: %w", err)
	}
	return nil
}

func relaunch() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Println("  update installed — please restart au")
		os.Exit(0)
	}
	fmt.Println("  restarting...")
	if err := execReplace(exe); err != nil {
		fmt.Printf("  restart failed: %v — please restart manually\n", err)
		os.Exit(1)
	}
}

func updateCmd() {
	fmt.Printf("  checking for updates (current: %s)...\n", version)
	tag, dlURL, err := checkUpdate()
	if err != nil {
		fmt.Printf("  \033[31merror\033[0m  %s\n", err)
		return
	}
	if !isNewer(tag, version) {
		fmt.Printf("  already up to date (%s)\n", version)
		return
	}
	if dlURL == "" {
		fmt.Printf("  new version %s available but no binary for %s/%s\n", tag, runtime.GOOS, runtime.GOARCH)
		fmt.Printf("  build from source: https://github.com/%s/%s\n", repoOwner, repoName)
		return
	}
	fmt.Printf("  updating %s → %s\n", version, tag)

	// Restore terminal before selfUpdate so sudo password prompt renders cleanly.
	ui.Teardown()

	if err := selfUpdate(dlURL); err != nil {
		fmt.Printf("  \033[31merror\033[0m  %s\n", err)
		return
	}
	fmt.Printf("  \033[32m✓\033[0m  updated to %s\n", tag)
	relaunch()
}
