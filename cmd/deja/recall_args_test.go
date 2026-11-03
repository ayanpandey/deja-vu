package main

import (
	"strings"
	"testing"
)

// The words after -- were put in front of the ones typed before it, so
// `deja recall foo -- --bar` asked for "--bar foo", and a single-dash flag
// such as `-n 5` was searched for as two query words (#5907).
func TestRecallArgsKeepTheQueryInOrder(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"foo", "--", "--bar"}, "foo --bar"},
		{[]string{"foo", "baz", "--", "--bar", "qux"}, "foo baz --bar qux"},
		{[]string{"foo", "--limit", "2", "--", "--bar"}, "foo --bar"},
		{[]string{"--", "--bar", "", "   ", "baz"}, "--bar baz"},
		{[]string{"foo", "--", ""}, "foo"},
		{[]string{"a", "-", "b"}, "a - b"},
	} {
		opts, err := parseRecallArgs(tc.args)
		if err != nil {
			t.Errorf("recall %q: %v", tc.args, err)
			continue
		}
		if opts.q != tc.want {
			t.Errorf("recall %q asked for %q, want %q", tc.args, opts.q, tc.want)
		}
	}

	for _, bad := range [][]string{{"-n", "5", "foo"}, {"foo", "-x"}, {"--", ""}, {"--", "   "}} {
		_, err := parseRecallArgs(bad)
		if err == nil {
			t.Errorf("recall %q was accepted", bad)
			continue
		}
		if strings.HasPrefix(bad[0], "-") && bad[0] != "--" && !strings.Contains(err.Error(), bad[0]) {
			t.Errorf("recall %q: the refusal does not name the flag: %v", bad, err)
		}
	}
}
