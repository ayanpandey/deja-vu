package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/vshulcz/deja-vu/internal/policy"
	"github.com/vshulcz/deja-vu/internal/search"
	"github.com/vshulcz/deja-vu/internal/termwidth"
	"github.com/vshulcz/deja-vu/internal/usage"
)

var recallFlags = []string{"--project", "--harness", "--limit", "--offset"}

// recallOptions is what `deja recall` was asked.
type recallOptions struct {
	q                string
	limit, offset    int
	project, harness string
}

// parseRecallArgs reads the `deja recall` arguments.
func parseRecallArgs(args []string) (recallOptions, error) {
	limit, offset := 0, 0
	project, harness := "", ""
	var terms, rest []string
	value := func(i int, flag string) (string, error) {
		if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
			return "", fmt.Errorf("recall: %s needs a value", flag)
		}
		return args[i+1], nil
	}
	// --flag=value splits into the two-argument form, and after -- every
	// word is the query, so a query may start with a dash. Those words join
	// the query after the ones typed before --, so `recall foo -- --bar`
	// asks for "foo --bar", not "--bar foo".
	var norm []string
	for i, a := range args {
		if a == "--" {
			for _, w := range args[i+1:] {
				if strings.TrimSpace(w) != "" {
					rest = append(rest, w)
				}
			}
			break
		}
		if k, v, ok := strings.Cut(a, "="); ok && strings.HasPrefix(k, "--") {
			norm = append(norm, k, v)
			continue
		}
		norm = append(norm, a)
	}
	args = norm
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--limit":
			v, err := value(i, a)
			if err != nil {
				return recallOptions{}, err
			}
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				return recallOptions{}, fmt.Errorf("recall: --limit wants a positive number, got %q", v)
			}
			limit, i = n, i+1
		case "--offset":
			// The page ends by naming the offset of the rest; a plugin tool that
			// serves this page needs a way to ask for it.
			v, err := value(i, a)
			if err != nil {
				return recallOptions{}, err
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return recallOptions{}, fmt.Errorf("recall: --offset wants a number from 0 up, got %q", v)
			}
			offset, i = n, i+1
		case "--project":
			v, err := value(i, a)
			if err != nil {
				return recallOptions{}, err
			}
			project, i = v, i+1
		case "--harness":
			v, err := value(i, a)
			if err != nil {
				return recallOptions{}, err
			}
			harness, i = v, i+1
		default:
			// A single dash is a flag too: `recall -n 5 foo` searched for
			// "-n 5 foo". A query that starts with a dash goes after --.
			if strings.HasPrefix(a, "-") && a != "-" {
				return recallOptions{}, unknownFlag("recall", a, recallFlags)
			}
			if strings.TrimSpace(a) != "" {
				terms = append(terms, a)
			}
		}
	}
	q := strings.TrimSpace(strings.Join(append(terms, rest...), " "))
	if q == "" {
		return recallOptions{}, errors.New("recall: what to recall? deja recall <words>")
	}
	return recallOptions{q: q, limit: limit, offset: offset, project: project, harness: harness}, nil
}

// runRecall prints what the MCP recall tool answers: the framed page, budgeted
// the same way, this project first. `deja search --json` was the only answer
// an agent could ask for on the command line, and it ran to 1-2 MB, which a
// harness truncates to nothing; a skill that drives deja through the shell had
// nothing it could read (#4781).
func runRecall(dir string, args []string, stdout io.Writer) error {
	opts, err := parseRecallArgs(args)
	if err != nil {
		return err
	}
	q, limit, offset, project, harness := opts.q, opts.limit, opts.offset, opts.project, opts.harness
	if err := checkHarness(&harness); err != nil {
		return err
	}
	if line := buildingNowForAgent(dir); line != "" {
		_, err := fmt.Fprintln(stdout, frameRecall(line))
		return err
	}
	text, sessions, raw, ids, projects, err := recallTextResultIn(dir, q, harness, project, limit, offset, recallMCPBudget-recallFrameOverhead)
	if err != nil {
		return err
	}
	text = frameRecall(text)
	usage.RecordServedFromInto(dir, usage.KindRecall, text, "", sessions, raw, ids, projects, policy.Load().Describe(policy.ActivationMCP))
	// Only a terminal gets the screen form: printableWidth answers COLUMNS for
	// a pipe too, and a plugin tool that inherited it handed the model a
	// rewrapped page with the frame's own line cut in four (#5568).
	if f, ok := stdout.(*os.File); ok && briefWanted(f) {
		if color, width := search.ColorOK(stdout), printableWidth(stdout); color || width > 0 {
			text = recallForScreen(text, color, width)
		}
	}
	_, err = fmt.Fprintln(stdout, text)
	return err
}

// recallHitHead is a numbered hit line of the recall page:
// "1. [claude] payments · a1f2c93b · 3 matches · updated 2025-11-29 (Nov 29 2025)".
var (
	recallHitHead = regexp.MustCompile(`^(\d+\.) \[([A-Za-z0-9_.-]+)\](.*)$`)
	recallUpdated = regexp.MustCompile(` · updated \d{4}-\d{2}-\d{2} \(([^)]*)\)`)
)

// recallForScreen is the recall page as a person at a terminal reads it. A pipe
// gets exactly what the MCP tool hands an agent; a terminal gets the same
// words with the frame dimmed, each hit's number bold and its harness in its
// colour, the day in one form instead of two, and every line wrapped to the
// width at spaces.
func recallForScreen(text string, color bool, width int) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if m := recallHitHead.FindStringSubmatch(l); m != nil {
			rest := recallUpdated.ReplaceAllString(m[3], " · updated $1")
			if color {
				lines[i] = statBold + m[1] + statReset + " " + search.HarnessTag(m[2], true) + rest
			} else {
				lines[i] = m[1] + " [" + m[2] + "]" + rest
			}
			continue
		}
		frame := i < 2 || l == "</deja-recall>"
		l = termwidth.WrapText(l, width)
		if color && frame {
			l = statDim + l + statReset
		}
		lines[i] = l
	}
	return strings.Join(lines, "\n")
}
