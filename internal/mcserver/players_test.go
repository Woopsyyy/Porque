package mcserver

import (
	"reflect"
	"testing"
)

func TestParseOnlineNames(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "standard single",
			in:   "There are 1 of a max of 50 players online: Woopsyyy",
			want: []string{"Woopsyyy"},
		},
		{
			name: "standard multiple",
			in:   "There are 3 of a max of 20 players online: Steve, Alex, .BedrockGuy",
			want: []string{"Steve", "Alex", ".BedrockGuy"},
		},
		{
			name: "slash format",
			in:   "There are 2/50 players online: Steve, Alex",
			want: []string{"Steve", "Alex"},
		},
		{
			name: "color codes stripped",
			in:   "There are 1 of a max of 50 players online: §eWoopsyyy§r",
			want: []string{"Woopsyyy"},
		},
		{
			name: "doubled/echoed header is cut",
			in:   "There are 1 of a max of 50 players online: WoopsyyyThere are 1 of a max of 50 players online: Woopsyyy",
			want: []string{"Woopsyyy"},
		},
		{
			name: "nobody online",
			in:   "There are 0 of a max of 50 players online:",
			want: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseOnlineNames(c.in)
			if len(got) == 0 && len(c.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("parseOnlineNames(%q) = %#v, want %#v", c.in, got, c.want)
			}
		})
	}
}

func TestParseWhitelistNames(t *testing.T) {
	got := parseWhitelistNames("There are 2 whitelisted players: Steve, Alex")
	want := []string{"Steve", "Alex"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseWhitelistNames = %#v, want %#v", got, want)
	}
}
