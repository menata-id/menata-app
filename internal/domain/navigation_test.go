package domain

import "testing"

func TestHomeCardRoute(t *testing.T) {
	tests := []struct {
		name  string
		items []NavigationItem
		want  string
	}{
		{
			name:  "no items",
			items: nil,
			want:  "",
		},
		{
			name: "none marked",
			items: []NavigationItem{
				{ID: "nav_home", Route: "/home"},
				{ID: "nav_inbox", Route: "/approval-inbox"},
			},
			want: "",
		},
		{
			name: "one marked",
			items: []NavigationItem{
				{ID: "nav_home", Route: "/home"},
				{ID: "nav_inbox", Route: "/approval-inbox", HomeCard: true},
			},
			want: "/approval-inbox",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HomeCardRoute(tt.items); got != tt.want {
				t.Errorf("HomeCardRoute() = %q, want %q", got, tt.want)
			}
		})
	}
}
