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
	version   = "v0.3.0-alpha"
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

	tmp := exe + ".new"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("cannot write update: %w", err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("download failed: %w", err)
	}
	f.Close()

	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("cannot replace binary: %w", err)
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Rename(old, exe) // restore on failure
		return fmt.Errorf("cannot install update: %w", err)
	}
	os.Remove(old)
	return nil
}

func relaunch() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Println("  update installed — please restart au")
		return
	}
	fmt.Println("  restarting...")
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Printf("  restart failed: %v — please restart manually\n", err)
		return
	}
	os.Exit(0)
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
	if err := selfUpdate(dlURL); err != nil {
		fmt.Printf("  \033[31merror\033[0m  %s\n", err)
		return
	}
	fmt.Printf("  \033[32m✓\033[0m  updated to %s\n", tag)
	ui.Teardown()
	relaunch()
}
