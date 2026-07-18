package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/emphity1/reap/internal/rules"
)

// Text renders findings as human-readable terminal output: grouped by file,
// then object, one block per finding, messages wrapped with a hanging
// indent. Color adds ANSI severity colors (the caller decides based on TTY,
// NO_COLOR, and the -color flag); Width is the wrap column (default 100).
type Text struct {
	Color bool
	Width int
}

const defaultWidth = 100

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiCyan   = "\x1b[36m"
)

func (t Text) Report(w io.Writer, res Result) error {
	if len(res.Findings) == 0 {
		_, err := fmt.Fprintf(w, "reap: %s checked, no findings%s\n",
			plural(res.ObjectsChecked, "object"), baselineNote(res))
		return err
	}
	width := t.Width
	if width <= 0 {
		width = defaultWidth
	}
	counts := map[rules.Severity]int{}
	lastSource, lastObject := "", ""
	for _, f := range res.Findings {
		counts[f.Severity]++
		if f.Source != lastSource {
			fmt.Fprintln(w, t.paint(ansiBold, f.Source))
			lastSource = f.Source
			lastObject = ""
		}
		if f.ObjectRef != lastObject {
			fmt.Fprintf(w, "  %s\n", t.paint(ansiCyan, f.ObjectRef))
			lastObject = f.ObjectRef
		}
		fmt.Fprintf(w, "    %s %s\n", t.severityTag(f.Severity), f.RuleID)
		for _, line := range wrap(f.Message, width-8) {
			fmt.Fprintf(w, "        %s\n", line)
		}
		if f.Fix != "" {
			lines := wrap(f.Fix, width-13)
			fmt.Fprintf(w, "        %s %s\n", t.paint(ansiDim, "fix:"), lines[0])
			for _, line := range lines[1:] {
				fmt.Fprintf(w, "             %s\n", line)
			}
		}
		fmt.Fprintln(w)
	}
	_, err := fmt.Fprintf(w, "reap: %s checked, %s (%s, %s, %s)%s\n",
		plural(res.ObjectsChecked, "object"), plural(len(res.Findings), "finding"),
		t.count(counts[rules.Error], "error", ansiRed),
		t.count(counts[rules.Warning], "warning", ansiYellow),
		t.count(counts[rules.Info], "info", ansiCyan),
		baselineNote(res))
	return err
}

func (t Text) severityTag(s rules.Severity) string {
	tag := "[" + s.String() + "]"
	switch s {
	case rules.Error:
		return t.paint(ansiRed, tag)
	case rules.Warning:
		return t.paint(ansiYellow, tag)
	default:
		return t.paint(ansiCyan, tag)
	}
}

func (t Text) count(n int, label, color string) string {
	s := fmt.Sprintf("%d %s", n, label)
	if n == 0 {
		return s
	}
	return t.paint(color, s)
}

func (t Text) paint(color, s string) string {
	if !t.Color {
		return s
	}
	return color + s + ansiReset
}

// baselineNote renders the baseline suffix of the summary line; empty when
// no baseline was in play.
func baselineNote(res Result) string {
	if res.Suppressed == 0 && res.StaleBaseline == 0 {
		return ""
	}
	note := fmt.Sprintf(" (%d suppressed by baseline", res.Suppressed)
	if res.StaleBaseline > 0 {
		note += fmt.Sprintf(", %s", plural(res.StaleBaseline, "stale entry"))
	}
	return note + ")"
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	if strings.HasSuffix(noun, "y") {
		return fmt.Sprintf("%d %sies", n, strings.TrimSuffix(noun, "y"))
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// wrap greedily fills lines up to width columns; words longer than the
// width get their own line rather than being split.
func wrap(s string, width int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	line := words[0]
	for _, word := range words[1:] {
		if len(line)+1+len(word) > width {
			lines = append(lines, line)
			line = word
			continue
		}
		line += " " + word
	}
	return append(lines, line)
}
