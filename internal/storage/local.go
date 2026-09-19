// Package storage saves and serves uploaded files on local disk (ROADMAP.md Phase 11). Matches
// 007-composable-runtime-architecture.md §4.10's single-binary, modest-server constraint -- no
// object-storage dependency until a real scale case forces one.
package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var unsafeChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// Store roots every file under one directory.
type Store struct {
	root string
}

// NewStore returns a Store rooted at root, creating it if it doesn't exist.
func NewStore(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create uploads dir: %w", err)
	}
	return &Store{root: root}, nil
}

// Save writes r to disk under a new key scoped to machineID/fieldID, returning that key. The
// original filename is preserved (sanitized) in the key itself -- "<random>__<filename>" -- so a
// later download can show it without a second field to carry it separately.
func (s *Store) Save(machineID, fieldID, filename string, r io.Reader) (string, error) {
	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return "", fmt.Errorf("generate file id: %w", err)
	}
	safe := unsafeChars.ReplaceAllString(filepath.Base(filename), "_")
	key := filepath.Join(machineID, fieldID, hex.EncodeToString(id)+"__"+safe)

	dest := filepath.Join(s.root, key)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", fmt.Errorf("create upload dir: %w", err)
	}
	f, err := os.Create(dest)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			log.Printf("close uploaded file %s: %v", dest, cerr)
		}
	}()
	if _, err := io.Copy(f, r); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return key, nil
}

// Path resolves key to an absolute path on disk, rejecting any key that would escape root
// (path traversal).
func (s *Store) Path(key string) (string, error) {
	root, err := filepath.Abs(s.root)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(filepath.Join(s.root, key))
	if err != nil {
		return "", err
	}
	if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid key %q", key)
	}
	return abs, nil
}

// DisplayName extracts the original filename from a key produced by Save.
func DisplayName(key string) string {
	base := filepath.Base(key)
	if idx := strings.Index(base, "__"); idx != -1 {
		return base[idx+2:]
	}
	return base
}
