package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	githubReleasesAPI = "https://api.github.com/repos/elvonpiko/tunnd/releases/latest"
	updateCheckTTL    = 24 * time.Hour
	installShURL      = "https://raw.githubusercontent.com/elvonpiko/tunnd/main/install.sh"
	installPS1URL     = "https://raw.githubusercontent.com/elvonpiko/tunnd/main/install.ps1"
)

// ── Subcommand ────────────────────────────────────────────────────────────────

func updateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Check for and install the latest tunnd client",
		Long: `Update tunnd to the latest released version.

Calls the official install script for your platform — same path used
for first install. If you're already on the latest version, this is a
no-op.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runUpdate()
		},
	}
}

func runUpdate() error {
	fmt.Println()
	fmt.Println("  Checking for updates…")

	latest, err := fetchLatestVersion(context.Background())
	if err != nil {
		return fmt.Errorf("could not check latest version: %w", err)
	}

	current := strings.TrimPrefix(Version, "v")
	latestClean := strings.TrimPrefix(latest, "v")
	fmt.Printf("  Current: %s\n", current)
	fmt.Printf("  Latest:  %s\n", latestClean)

	if current == latestClean {
		fmt.Println()
		fmt.Println("  ✓ Already on the latest version.")
		fmt.Println()
		return nil
	}

	fmt.Println()
	fmt.Println("  Updating…")

	if err := runInstaller(); err != nil {
		return fmt.Errorf("running installer: %w", err)
	}

	fmt.Println()
	fmt.Printf("  ✓ Updated to %s\n", latestClean)
	fmt.Println()
	return nil
}

// ── Passive update hint ───────────────────────────────────────────────────────

// maybePrintUpdateHint prints a single-line "v0.1.x is available" notice
// when a newer release exists. The check is cached for 24h in
// ~/.config/tunnd/update-check.json so we never make more than one
// network call per day. Network failure is silent — the hint is purely
// nice-to-have and must never block or noise up the tunnel UX.
func maybePrintUpdateHint() {
	if Version == "dev" {
		return // local builds shouldn't nag
	}
	if strings.EqualFold(os.Getenv("TUNND_NO_UPDATE_CHECK"), "1") {
		return
	}

	state, _ := readUpdateState()
	if time.Since(state.LastChecked) < updateCheckTTL {
		// Use the cached "latest" if we have one; only warn when newer.
		if state.LatestVersion != "" && newerThan(state.LatestVersion, Version) {
			printHint(state.LatestVersion)
		}
		return
	}

	// Out of cache — refresh in background so we don't slow down banner output.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		latest, err := fetchLatestVersion(ctx)
		if err != nil {
			return
		}
		_ = writeUpdateState(updateState{
			LastChecked:   time.Now(),
			LatestVersion: latest,
		})
	}()

	// Use whatever cached version we already have to decide on this run.
	if state.LatestVersion != "" && newerThan(state.LatestVersion, Version) {
		printHint(state.LatestVersion)
	}
}

func printHint(latest string) {
	fmt.Printf("  ⓘ  %s is available — run `tunnd update` to upgrade.\n", strings.TrimPrefix(latest, "v"))
	fmt.Println()
}

// ── Network ───────────────────────────────────────────────────────────────────

type ghRelease struct {
	TagName string `json:"tag_name"`
}

// fetchLatestVersion queries the GitHub Releases API and returns the
// latest tag (e.g. "v0.1.1"). The supplied context bounds the call.
func fetchLatestVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubReleasesAPI, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "tunnd/"+Version)

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github releases api: HTTP %d", resp.StatusCode)
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("github releases api returned no tag_name")
	}
	return rel.TagName, nil
}

// ── Installer shellout ────────────────────────────────────────────────────────

// runInstaller invokes the right install script for the user's platform.
// On Windows we shell out to PowerShell with the install.ps1 URL. On
// Linux/macOS we use sh. Stdout and stderr are inherited so the user
// sees live progress.
func runInstaller() error {
	switch runtime.GOOS {
	case "windows":
		// PowerShell can run a remote script via iwr | iex.
		ps := fmt.Sprintf("iwr -useb %s | iex", installPS1URL)
		cmd := exec.Command("powershell", "-NoProfile", "-Command", ps) //nolint:gosec
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()

	case "linux", "darwin":
		// curl -fsSL <url> | sh
		shCmd := fmt.Sprintf("curl -fsSL %s | bash", installShURL)
		cmd := exec.Command("sh", "-c", shCmd) //nolint:gosec
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()

	default:
		return fmt.Errorf("automatic update is not supported on %s — download from https://github.com/elvonpiko/tunnd/releases", runtime.GOOS)
	}
}

// ── Cache ─────────────────────────────────────────────────────────────────────

type updateState struct {
	LastChecked   time.Time `json:"last_checked"`
	LatestVersion string    `json:"latest_version"`
}

func updateStatePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "tunnd", "update-check.json")
}

func readUpdateState() (updateState, error) {
	var s updateState
	data, err := os.ReadFile(updateStatePath())
	if err != nil {
		return s, err
	}
	_ = json.Unmarshal(data, &s)
	return s, nil
}

func writeUpdateState(s updateState) error {
	path := updateStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// ── Version comparison ────────────────────────────────────────────────────────

// newerThan reports whether candidate (e.g. "v0.1.2") is strictly greater
// than current (e.g. "v0.1.1"). Both inputs may carry the leading "v".
// Falls back to string compare on anything that isn't a clean
// "major.minor.patch" — good enough for our linear release cadence
// without pulling in a semver dependency for one comparison.
func newerThan(candidate, current string) bool {
	c := splitSemver(strings.TrimPrefix(candidate, "v"))
	u := splitSemver(strings.TrimPrefix(current, "v"))
	if c == nil || u == nil {
		return candidate != current
	}
	for i := 0; i < 3; i++ {
		if c[i] > u[i] {
			return true
		}
		if c[i] < u[i] {
			return false
		}
	}
	return false
}

func splitSemver(s string) []int {
	parts := strings.Split(s, ".")
	if len(parts) < 3 {
		return nil
	}
	out := make([]int, 3)
	for i := 0; i < 3; i++ {
		// Trim any pre-release / build metadata after the first non-digit.
		end := 0
		for end < len(parts[i]) && parts[i][end] >= '0' && parts[i][end] <= '9' {
			end++
		}
		if end == 0 {
			return nil
		}
		var n int
		for j := 0; j < end; j++ {
			n = n*10 + int(parts[i][j]-'0')
		}
		out[i] = n
	}
	return out
}
