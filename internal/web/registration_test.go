package web

import "testing"

func TestSlugify(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Acme Corp", "acme-corp"},
		{"  Spaces   Everywhere  ", "spaces-everywhere"},
		{"Special!! Ch@rs***", "special-ch-rs"},
		{"", "workspace"},
		{"!!!", "workspace"},
		{"already-a-slug", "already-a-slug"},
	}
	for _, c := range cases {
		if got := slugify(c.in); got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidateRegistration(t *testing.T) {
	cases := []struct {
		name, workspaceName, fullName, email, password string
		wantOK                                         bool
	}{
		{"valid", "Acme", "Ana Putri", "a@example.com", "hunter22", true},
		{"missing workspace name", "", "Ana Putri", "a@example.com", "hunter22", false},
		{"missing full name", "Acme", "", "a@example.com", "hunter22", false},
		{"missing email", "Acme", "Ana Putri", "", "hunter22", false},
		{"short password", "Acme", "Ana Putri", "a@example.com", "short", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ok := validateRegistration(c.workspaceName, c.fullName, c.email, c.password)
			if ok != c.wantOK {
				t.Errorf("validateRegistration(%q, %q, %q, %q) ok = %v, want %v", c.workspaceName, c.fullName, c.email, c.password, ok, c.wantOK)
			}
		})
	}
}
