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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
)

type previewMsg struct {
	key     string
	content string
}

func previewKey(pr prItem, width int) string {
	return pr.URL + "|" + strconv.Itoa(width) + "|" + strconv.FormatInt(pr.UpdatedAt.Unix(), 10)
}

func renderPreviewCmd(pr prItem, width int) tea.Cmd {
	key := previewKey(pr, width)
	body := pr.Body
	return func() tea.Msg {
		content, err := renderMarkdown(body, width)
		if err != nil {
			content = body // raw markdown beats nothing
		}
		return previewMsg{key: key, content: content}
	}
}

// glamourStyle is decided once in main() BEFORE the bubbletea program starts.
// glamour's WithAutoStyle queries the terminal for its background color; done
// after startup that reply races bubbletea's input reader and ends up typed
// into the filter as literal "rgb:..." text.
var glamourStyle = "dark"

func renderMarkdown(body string, width int) (string, error) {
	if strings.TrimSpace(body) == "" {
		return stDim.Render("(no description)"), nil
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(glamourStyle),
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
