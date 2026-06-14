package playit

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestSidecarName(t *testing.T) {
	if got := sidecarName("survival"); got != "mc-playit-survival" {
		t.Errorf("sidecarName = %q", got)
	}
	if got := sidecarName("My Awesome Server!"); got != "mc-playit-my-awesome-server" {
		t.Errorf("sidecarName = %q", got)
	}
}

func TestStatusTopic(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	if got := statusTopic(id); got != "playit:11111111-1111-1111-1111-111111111111" {
		t.Errorf("statusTopic = %q", got)
	}
}

func TestStubClient_ReturnsNoTunnels(t *testing.T) {
	got, err := StubClient{}.ListTunnels(context.Background(), "any-secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no tunnels, got %d", len(got))
	}
}
