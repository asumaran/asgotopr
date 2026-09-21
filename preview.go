package main

// PR description preview: glamour renders the markdown body to ANSI for the
// right-hand column. Rendering a large body can take tens of milliseconds, so
// it runs as a tea.Cmd and results are cached per (URL, width, updatedAt) —
// the updatedAt component makes stale renders unreachable after a refresh
// without explicit invalidation.

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func previewKey(pr prItem, width int) string {
	return pr.URL + "|" + strconv.Itoa(width) + "|" + strconv.FormatInt(pr.UpdatedAt.Unix(), 10)
}

func renderPreviewCmd(pr prItem, width int, style string) tea.Cmd {
	key := previewKey(pr, width)
	body := pr.Body
	return func() tea.Msg {
		content, err := renderMarkdown(body, width, style)
		switch {
		case err != nil:
			content = body // raw text beats nothing
		case content == "":
			content = stDim.Render("(no description)")
		}
		return previewMsg{key: key, style: style, content: content}
	}
}

// previewHeader is the instant (non-glamour) block above the rendered body:
// title, refs, label chips, then aligned facts (checks, review, diff, dates).
// rightColumn puts one blank line under it, as in every tool of the family
// with a header. Its height varies per PR; syncPreviewHeight fits the body
// viewport under it.
func previewHeader(pr prItem, width int) string {
	lines := []string{stTitle.Render(truncate(pr.Title, width))}

	ref := "#" + strconv.Itoa(pr.Number) + " · " + pr.HeadRefName
	if pr.BaseRefName != "" {
		ref += " → " + pr.BaseRefName
	}
	refLine := stDim.Render(ref)
	if pr.IsDraft {
		refLine += " " + stBadgeDraft.Render("DRAFT")
	}
	if pr.Mergeable == "CONFLICTING" {
		refLine += " " + stBadgeBad.Render("CONFLICTS")
	}
	lines = append(lines, truncate(refLine, width))

	if len(pr.Labels) > 0 {
		// Chips paint a background; a blank line keeps them from crowding
		// the refs line above.
		lines = append(lines, "")
		lines = append(lines, wrapChips(labelChips(pr.Labels), width)...)
	}

	lines = append(lines, "")
	lines = append(lines, fact("Checks", checksLine(pr.Checks), width))
	if names := pr.Checks.FailedNames; len(names) > 0 {
		lines = append(lines, fact("", stDim.Render("↳ "+strings.Join(names, ", ")), width))
	}
	lines = append(lines, fact("Review", reviewLine(pr), width))
	if pr.ChangedFiles > 0 || pr.Commits > 0 {
		lines = append(lines, fact("Diff", diffLine(pr), width))
	}
	lines = append(lines, fact("Opened", openedLine(pr), width))
	return strings.Join(lines, "\n")
}

const factKeyW = 8

// fact renders one "Key    value" row of the header.
func fact(key, value string, width int) string {
	k := key + strings.Repeat(" ", max(0, factKeyW-len(key)))
	return truncate(stFactKey.Render(k)+value, width)
}

// labelChips renders each label on its GitHub color with a readable
// foreground. Unparseable colors fall back to a plain dim chip.
func labelChips(labels []prLabel) []string {
	chips := make([]string, 0, len(labels))
	for _, l := range labels {
		st := lipgloss.NewStyle().Padding(0, 1)
		if fg, ok := chipForeground(l.Color); ok {
			st = st.Background(lipgloss.Color("#" + strings.ToLower(l.Color))).Foreground(lipgloss.Color(fg))
		} else {
			st = st.Foreground(lipgloss.Color("8")).Reverse(true)
		}
		chips = append(chips, st.Render(l.Name))
	}
	return chips
}

// chipForeground picks black or white text for a 6-digit hex background by
// perceived luminance.
func chipForeground(hex string) (string, bool) {
	if len(hex) != 6 {
		return "", false
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return "", false
	}
	r, g, b := float64(v>>16&0xff), float64(v>>8&0xff), float64(v&0xff)
	if 0.299*r+0.587*g+0.114*b > 150 {
		return "#000000", true
	}
	return "#ffffff", true
}

// wrapChips lays chips out left to right, breaking lines at width.
func wrapChips(chips []string, width int) []string {
	var lines []string
	cur, curW := "", 0
	for _, c := range chips {
		cw := ansi.StringWidth(c)
		if cur != "" && curW+1+cw > width {
			lines = append(lines, cur)
			cur, curW = "", 0
		}
		if cur != "" {
			cur += " "
			curW++
		}
		cur += c
		curW += cw
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

func checksLine(c checkSummary) string {
	if c.State == "" || c.Total == 0 {
		return stDim.Render("none")
	}
	var parts []string
	if c.Failed > 0 {
		parts = append(parts, stBad.Render(fmt.Sprintf("✗ %d failed", c.Failed)))
	}
	if c.Pending > 0 {
		parts = append(parts, stWarn.Render(fmt.Sprintf("● %d pending", c.Pending)))
	}
	if c.Passed > 0 {
		parts = append(parts, stOK.Render(fmt.Sprintf("✓ %d passed", c.Passed)))
	}
	if c.Skipped > 0 {
		parts = append(parts, stDim.Render(fmt.Sprintf("%d skipped", c.Skipped)))
	}
	if len(parts) == 0 {
		// Rollup exists but the contexts were all beyond the first page.
		return stDim.Render(strings.ToLower(c.State))
	}
	return strings.Join(parts, stDim.Render(" · "))
}

func reviewLine(pr prItem) string {
	var head string
	switch pr.ReviewDecision {
	case "APPROVED":
		head = stOK.Render("✓ approved")
	case "CHANGES_REQUESTED":
		head = stBad.Render("✗ changes requested")
	case "REVIEW_REQUIRED":
		head = stWarn.Render("○ review required")
	default: // no review rule on the base branch
		if pr.Approvals > 0 {
			head = stOK.Render("✓ approved")
		} else {
			head = stDim.Render("not reviewed")
		}
	}
	parts := []string{head}
	if pr.Approvals > 0 {
		parts = append(parts, plural(pr.Approvals, "approval"))
	}
	if pr.ReviewRequests > 0 {
		parts = append(parts, plural(pr.ReviewRequests, "reviewer")+" pending")
	}
	if pr.Comments > 0 {
		parts = append(parts, plural(pr.Comments, "comment"))
	}
	return strings.Join(parts, stDim.Render(" · "))
}

func diffLine(pr prItem) string {
	parts := []string{
		stOK.Render("+"+strconv.Itoa(pr.Additions)) + " " + stBad.Render("−"+strconv.Itoa(pr.Deletions)),
		plural(pr.ChangedFiles, "file"),
	}
	if pr.Commits > 0 {
		parts = append(parts, plural(pr.Commits, "commit"))
	}
	return strings.Join(parts, stDim.Render(" · "))
}

func openedLine(pr prItem) string {
	var parts []string
	if !pr.CreatedAt.IsZero() {
		s := relTime(pr.CreatedAt)
		if pr.Author != "" {
			s += " by " + pr.Author
		}
		parts = append(parts, s)
	}
	parts = append(parts, "updated "+relTime(pr.UpdatedAt))
	return strings.Join(parts, stDim.Render(" · "))
}
