package config

import "testing"

func TestParseWorkers(t *testing.T) {
	ws, err := ParseWorkers(" w1=http://a:1/ , w2=http://b:2 ")
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 2 || ws[0] != (Worker{"w1", "http://a:1"}) || ws[1] != (Worker{"w2", "http://b:2"}) {
		t.Fatalf("unexpected workers: %+v", ws)
	}

	for _, bad := range []string{"", "noequals", "=http://x", "id="} {
		if _, err := ParseWorkers(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}
