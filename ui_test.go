package main

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func testModel(t *testing.T) model {
	t.Helper()
	prs := []prItem{
		{URL: "u1", Number: 100, Title: "fix login flow", HeadRefName: "fix/login",
			RepoSlug: "org/alpha", UpdatedAt: time.Unix(300, 0), Body: "Some **body** text"},
		{URL: "u2", Number: 7, Title: "update readme", HeadRefName: "docs/readme",
			RepoSlug: "org/beta", UpdatedAt: time.Unix(100, 0)},
	}
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Focus()
	m := model{
		slugs: map[string][]localRepo{
			"org/alpha": {{Slug: "org/alpha", Path: "/d/alpha", Name: "alpha"}},
			"org/beta":  {{Slug: "org/beta", Path: "/d/beta", Name: "beta"}},
		},
		cache:   prCache{FetchedAt: time.Now(), PRs: prs},
		ti:      ti,
		listVP:  viewport.New(viewport.WithWidth(50), viewport.WithHeight(20)),
		prevVP:  viewport.New(viewport.WithWidth(40), viewport.WithHeight(17)),
		help:    help.New(),
		keys:    defaultKeys(),
		renders: map[string]string{},
		width:   120,
		height:  30,
	}
	m.setEntries(prs)
	m.applyFilter()
	m.resize()
	m.renderList()
	return m
}

func TestViewRendersRows(t *testing.T) {
	m := testModel(t)
	view := m.View().Content
	for _, want := range []string{"alpha", "#100", "fix login flow", "beta", "update readme"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() missing %q\n----\n%s", want, view)
		}
	}
	if !strings.Contains(view, "fix login flow") {
		t.Errorf("preview header missing selected PR title")
	}
}

func TestViewAfterWindowResize(t *testing.T) {
	m := testModel(t)
	res, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 34})
	view := res.(model).View().Content
	if !strings.Contains(view, "#100") {
		t.Errorf("View() after resize missing rows:\n%s", view)
	}
}

func TestFilterNarrowsRows(t *testing.T) {
	m := testModel(t)
	var mm tea.Model = m
	for _, r := range "readme" {
		mm, _ = mm.(model).handleKey(keyMsg(string(r)))
	}
	view := mm.(model).View().Content
	if strings.Contains(view, "fix login flow") {
		t.Errorf("filter kept non-matching PR:\n%s", view)
	}
	if !strings.Contains(view, "update readme") {
		t.Errorf("filter lost matching PR:\n%s", view)
	}
}

func TestBackgroundColorFlipsPreviewStyle(t *testing.T) {
	m := testModel(t)
	m.renders["stale"] = "old palette"
	res, cmd := m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")})
	got := res.(model)
	if got.previewStyle != "light" {
		t.Errorf("light background: style = %q", got.previewStyle)
	}
	if _, ok := got.renders["stale"]; ok {
		t.Errorf("style change kept the old render cache")
	}
	if cmd == nil {
		t.Errorf("style change did not re-render the current preview")
	}
	// A render produced under the old style is dropped.
	res, _ = got.Update(previewMsg{key: got.prevKey, style: "dark", content: "dark render"})
	if c := res.(model).renders[got.prevKey]; c != "" {
		t.Errorf("stale-style render was cached: %q", c)
	}
	// The same answer again is a no-op.
	res, cmd = got.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")})
	if cmd != nil || res.(model).previewStyle != "light" {
		t.Errorf("repeated background reply was not a no-op")
	}
	// A dark background keeps the default.
	res, _ = testModel(t).Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#000000")})
	if res.(model).previewStyle != "dark" {
		t.Errorf("dark background: style = %q", res.(model).previewStyle)
	}
}
