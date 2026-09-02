package update

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		cur, latest string
		want        int
	}{
		{"v0.1.0", "v0.1.0", CmpEQ},
		{"0.0.10-SNAPSHOT-b3a96b5e", "v0.1.0", CmpLT},
		{"v0.1.0", "v0.1.1", CmpLT},
		{"v0.2.0", "v0.1.9", CmpGT},
		{"v0.1.0-SNAPSHOT", "v0.1.0", CmpLT},
		{"v0.9.9", "v0.10.0", CmpLT},
		{"v0.1.0", "v0.2.0-alpha", CmpLT},
		{"v1.0.0", "v0.99.99", CmpGT},
	}
	for _, c := range cases {
		if got := CompareVersions(c.cur, c.latest); got != c.want {
			t.Errorf("CompareVersions(%q, %q)=%d, want %d", c.cur, c.latest, got, c.want)
		}
	}
}