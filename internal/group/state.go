package group

import (
	"os"
	"path/filepath"
)

func writeFile(dir, name string, data []byte) error {
	return os.WriteFile(filepath.Join(dir, name), data, 0644)
}

func readFile(dir, name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(dir, name))
}
