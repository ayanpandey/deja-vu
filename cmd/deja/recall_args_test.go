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
		want recallOptions
	}{
		{[]string{"foo", "--", "--bar"}, recallOptions{q: "foo --bar"}},
		{[]string{"foo", "baz", "--", "--bar", "qux"}, recallOptions{q: "foo baz --bar qux"}},
		{[]string{"foo", "--limit", "2", "--", "--bar"}, recallOptions{q: "foo --bar", limit: 2}},
		{[]string{"--limit", "2", "foo"}, recallOptions{q: "foo", limit: 2}},
		{[]string{"foo", "--limit=2"}, recallOptions{q: "foo", limit: 2}},
		{[]string{"--project=x", "foo", "--", "--bar"}, recallOptions{q: "foo --bar", project: "x"}},
		{[]string{"--harness=claude", "--offset=3", "foo", "--", "-n"}, recallOptions{q: "foo -n", harness: "claude", offset: 3}},
		{[]string{"--", "--bar", "", "   ", "baz"}, recallOptions{q: "--bar baz"}},
		{[]string{"foo", "", "   ", "bar"}, recallOptions{q: "foo bar"}},
		{[]string{"foo", "--", ""}, recallOptions{q: "foo"}},
		{[]string{"a", "-", "b"}, recallOptions{q: "a - b"}},
	} {
		got, err := parseRecallArgs(tc.args)
		if err != nil {
			t.Errorf("recall %q: %v", tc.args, err)
			continue
		}
		if got != tc.want {
			t.Errorf("recall %q parsed %+v, want %+v", tc.args, got, tc.want)
		}
	}

	for _, tc := range []struct {
		args []string
		in   string // text the refusal must contain
	}{
		{[]string{"-n", "5", "foo"}, `"-n"`},
		{[]string{"foo", "-x"}, `"-x"`},
		{[]string{"foo", "-5"}, `"-5"`},
		{[]string{"-n=5", "foo"}, `"-n=5"`},
		{[]string{"--limt", "2", "foo"}, "did you mean --limit"},
		{[]string{"foo", "--limit"}, "needs a value"},
		{[]string{"--", ""}, "what to recall"},
		{[]string{"--", "   "}, "what to recall"},
	} {
		_, err := parseRecallArgs(tc.args)
		if err == nil {
			t.Errorf("recall %q was accepted", tc.args)
			continue
		}
		if !strings.Contains(err.Error(), tc.in) {
			t.Errorf("recall %q: refusal %q does not contain %q", tc.args, err, tc.in)
		}
	}
}
