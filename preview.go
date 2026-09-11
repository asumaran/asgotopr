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
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
)

type previewMsg struct {
	key     string
	style   string // glamour style the render used; dropped if it changed since
	content string
}

func previewKey(pr prItem, width int) string {
	return pr.URL + "|" + strconv.Itoa(width) + "|" + strconv.FormatInt(pr.UpdatedAt.Unix(), 10)
}

func renderPreviewCmd(pr prItem, width int, style string) tea.Cmd {
	key := previewKey(pr, width)
	body := pr.Body
	return func() tea.Msg {
		content, err := renderMarkdown(body, width, style)
		if err != nil {
			content = body // raw markdown beats nothing
		}
		return previewMsg{key: key, style: style, content: content}
	}
}

// renderMarkdown renders with a fixed glamour standard style ("dark" or
// "light"). The style is never auto-detected here: bubbletea owns the
// terminal, so the model asks it for the background color (Init →
// RequestBackgroundColor) and passes the answer down.
func renderMarkdown(body string, width int, style string) (string, error) {
	if strings.TrimSpace(body) == "" {
		return stDim.Render("(no description)"), nil
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
	)
	if err != nil {
		return "", err
	}
	out, err := r.Render(body)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(out, "\n"), nil
}

// previewHeader is the instant (non-glamour) header above the rendered body.
func previewHeader(pr prItem, width int) string {
	meta := fmt.Sprintf("#%d · %s · updated %s", pr.Number, pr.HeadRefName, relTime(pr.UpdatedAt))
	if pr.IsDraft {
		meta += " · draft"
	}
	title := truncate(pr.Title, width)
	return stTitle.Render(title) + "\n" + stDim.Render(truncate(meta, width))
}

// relTime formats a timestamp as a compact "2h ago" style age.
func relTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
