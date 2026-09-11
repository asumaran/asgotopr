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
	// Each wheel event in the tests is its own gesture unless a test installs
	// its own clock.
	t0 := time.Unix(1000, 0)
	m.now = func() time.Time { t0 = t0.Add(time.Second); return t0 }
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

func wheelDown(x int) tea.MouseWheelMsg {
	return tea.MouseWheelMsg{X: x, Y: 5, Button: tea.MouseWheelDown}
}

func wheelUp(x int) tea.MouseWheelMsg {
	return tea.MouseWheelMsg{X: x, Y: 5, Button: tea.MouseWheelUp}
}

func TestMouseWheelScrollsColumnsIndependently(t *testing.T) {
	m := testModel(t)
	m.prevVP.SetContent(strings.Repeat("line\n", 100))
	first := m.cursor
	res, _ := m.Update(wheelDown(2))
	m = res.(model)
	if m.cursor == first || m.prevVP.YOffset() != 0 {
		t.Errorf("wheel over list: cursor=%d (was %d) preview=%d, want cursor moved and preview=0", m.cursor, first, m.prevVP.YOffset())
	}
	cur := m.cursor
	m.prevVP.SetContent(strings.Repeat("line\n", 100)) // selection change reset the preview
	res, _ = m.Update(wheelDown(m.listW() + 10))
	m = res.(model)
	if m.prevVP.YOffset() == 0 || m.cursor != cur {
		t.Errorf("wheel over preview: cursor=%d preview=%d, want cursor=%d preview>0", m.cursor, m.prevVP.YOffset(), cur)
	}
	res, _ = m.Update(wheelUp(2))
	if got := res.(model).cursor; got != first {
		t.Errorf("wheel up over list: cursor=%d, want back to %d", got, first)
	}
}

func TestMouseWheelBurstNeverTypesIntoFilter(t *testing.T) {
	m := testModel(t)
	m.prevVP.SetContent(strings.Repeat("line\n", 100))
	last := m.cursor
	for i := len(m.rows) - 1; i >= 0; i-- {
		if m.rows[i].kind == "pr" {
			last = i
			break
		}
	}
	var mm tea.Model = m
	for i := 0; i < 200; i++ {
		x := 2
		if i%2 == 1 {
			x = m.listW() + 10
		}
		mm, _ = mm.Update(wheelDown(x))
	}
	got := mm.(model)
	if got.ti.Value() != "" {
		t.Errorf("filter got mouse text %q", got.ti.Value())
	}
	if got.cursor != last {
		t.Errorf("burst did not move the selection to the last PR: cursor=%d want %d", got.cursor, last)
	}
}

func TestMouseWheelIgnoredOutsideFilterMode(t *testing.T) {
	m := testModel(t)
	m.listVP.SetContent(strings.Repeat("row\n", 100))
	m.mode = modeConfirmStash
	m.pending = m.currentRow().e
	before := m.cursor
	res, _ := m.Update(wheelDown(2))
	got := res.(model)
	if got.cursor != before {
		t.Errorf("wheel moved the selection in confirm mode: %d -> %d", before, got.cursor)
	}
	if got.View().MouseMode != tea.MouseModeNone {
		t.Errorf("confirm mode still requests mouse reports")
	}
	if testModel(t).View().MouseMode != tea.MouseModeCellMotion {
		t.Errorf("filter mode does not request mouse reports")
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

func TestWheelGestureStaysOnStartingColumn(t *testing.T) {
	m := testModel(t)
	m.prevVP.SetContent(strings.Repeat("line\n", 100))
	clock := time.Unix(1000, 0)
	m.now = func() time.Time { return clock }
	first := m.cursor
	// Gesture starts over the preview...
	res, _ := m.Update(wheelDown(m.listW() + 10))
	m = res.(model)
	// ...then inertial events arrive over the list 50ms apart: they must keep
	// scrolling the preview, not move the selection.
	for i := 0; i < 5; i++ {
		clock = clock.Add(50 * time.Millisecond)
		res, _ = m.Update(wheelDown(2))
		m = res.(model)
	}
	if m.cursor != first {
		t.Errorf("inertia spilled into the list: cursor %d -> %d", first, m.cursor)
	}
	if m.prevVP.YOffset() < 6*3 {
		t.Errorf("preview did not get the whole gesture: YOffset=%d", m.prevVP.YOffset())
	}
	// After a pause the next event starts a new gesture over the list.
	clock = clock.Add(wheelGestureGap + time.Millisecond)
	res, _ = m.Update(wheelDown(2))
	if got := res.(model).cursor; got == first {
		t.Errorf("new gesture over the list did not move the selection")
	}
}
