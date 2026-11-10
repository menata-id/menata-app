package config

import "testing"

// The server binds to loopback unless LISTEN_HOST says otherwise: Caddy fronts it, and a bind on every interface
// once made :4000 reachable from the internet over plain HTTP, past TLS and the proxy's rules.
func TestServerBindsToLoopbackByDefault(t *testing.T) {
	t.Setenv("LISTEN_HOST", "")
	if got := Load().ListenHost; got != "127.0.0.1" {
		t.Errorf("ListenHost = %q, want 127.0.0.1", got)
	}
	t.Setenv("LISTEN_HOST", "10.0.0.5")
	if got := Load().ListenHost; got != "10.0.0.5" {
		t.Errorf("ListenHost = %q, want the LISTEN_HOST it was given", got)
	}
}
