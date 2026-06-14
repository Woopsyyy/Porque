package rcon

import "testing"

func TestParsePlayerList(t *testing.T) {
	p, m, ok := ParsePlayerList("There are 3 of a max of 20 players online: alice, bob, carol")
	if !ok || p != 3 || m != 20 {
		t.Fatalf("got players=%d max=%d ok=%v, want 3/20/true", p, m, ok)
	}

	p, m, ok = ParsePlayerList("There are 0 of a max of 20 players online:")
	if !ok || p != 0 || m != 20 {
		t.Fatalf("empty server: got %d/%d ok=%v", p, m, ok)
	}

	if _, _, ok := ParsePlayerList("unrecognized output"); ok {
		t.Fatal("expected ok=false for unrecognized output")
	}
}
