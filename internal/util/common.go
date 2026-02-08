package util

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Common timeout durations
const (
	DefaultFetchTimeout   = 5 * time.Second
	DefaultConnectTimeout = 3 * time.Second
	ShortTimeout          = 2 * time.Second
)

// ResolvePath joins base and rel, but if rel is an absolute path it is returned
// directly (cleaned). Go's filepath.Join strips leading slashes from later
// arguments, so filepath.Join("a", "/b") returns "a/b" not "/b".  This helper
// gives the intuitive behaviour: absolute paths override the base.
func ResolvePath(base, rel string) string {
	if filepath.IsAbs(rel) {
		return filepath.Clean(rel)
	}
	return filepath.Join(base, rel)
}

// ValidatePeerName validates and normalizes a peer name.
// Returns the trimmed name and an error if invalid.
func ValidatePeerName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("peer name is empty")
	}
	if strings.ContainsAny(name, `/\ `) || strings.Contains(name, "..") {
		return "", errors.New("peer name must not contain spaces, slashes or '..'")
	}
	return name, nil
}

// WriteJSONFile writes a JSON object to a file, creating parent directories if needed.
func WriteJSONFile(path string, v any) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// OpenURL opens a URL in the system's default browser
func OpenURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return errors.New("unsupported platform")
	}
	return cmd.Start()
}
