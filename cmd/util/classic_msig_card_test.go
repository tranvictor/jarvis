package util

import "testing"

func TestClassicSummaryStatus(t *testing.T) {
	cases := []struct {
		confirmed int
		required  int64
		executed  bool
		want      string
	}{
		{0, 2, true, "executed"},
		{2, 2, false, "ready to execute"},
		{3, 2, false, "ready to execute"},
		{1, 2, false, "pending"},
		{0, 0, false, "pending"},
	}
	for _, c := range cases {
		got := ClassicSummaryStatus(c.confirmed, c.required, c.executed)
		if got != c.want {
			t.Errorf("ClassicSummaryStatus(%d, %d, %v) = %q, want %q",
				c.confirmed, c.required, c.executed, got, c.want)
		}
	}
}
