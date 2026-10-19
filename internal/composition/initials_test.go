package composition

import "testing"

// TestInitials_isRuneSafe: the first letter of a name is a character, not a byte -- a name outside ASCII must not become half a
// UTF-8 sequence.
func TestInitials_isRuneSafe(t *testing.T) {
	for name, want := range map[string]string{
		"Édith Piaf": "ÉP", "李雷 韩": "李韩", "budi": "B", "  ": "?", "": "?", "ñandú  óscar x": "ÑÓ",
	} {
		if got := Initials(name); got != want {
			t.Errorf("Initials(%q) = %q, want %q", name, got, want)
		}
	}
}
