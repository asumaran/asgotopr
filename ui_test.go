package main

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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
		listVP:  viewport.New(50, 20),
		prevVP:  viewport.New(40, 17),
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
	view := m.View()
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
	view := res.(model).View()
	if !strings.Contains(view, "#100") {
		t.Errorf("View() after resize missing rows:\n%s", view)
	}
}

func TestFilterNarrowsRows(t *testing.T) {
	m := testModel(t)
	var mm tea.Model = m
	for _, r := range "readme" {
		mm, _ = mm.(model).handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	view := mm.(model).View()
	if strings.Contains(view, "fix login flow") {
		t.Errorf("filter kept non-matching PR:\n%s", view)
	}
	if !strings.Contains(view, "update readme") {
		t.Errorf("filter lost matching PR:\n%s", view)
	}
}
