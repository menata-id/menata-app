package metadata

import (
	"fmt"
	"os"

	"menata.app/internal/domain"
)

// Load reads, parses, and validates a Runtime Metadata file describing a Machine
// (005-runtime-lifecycle.md Phase 3-4: parse, then validate before anything downstream
// consumes the result).
func Load(path string) (*domain.Machine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read metadata %s: %w", path, err)
	}

	m, err := Parse(data)
	if err != nil {
		return nil, err
	}

	if err := Validate(m); err != nil {
		return nil, err
	}

	return m, nil
}
