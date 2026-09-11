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
	"github.com/charmbracelet/x/ansi"
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

func TestSelectedRowSpansListWidth(t *testing.T) {
	m := testModel(t)
	w := m.listW()
	r := m.rows[m.cursor]
	got := m.rowLine(r, true, m.prNumW(), w)
	if n := ansi.StringWidth(got); n != w {
		t.Errorf("selected row width = %d, want %d: %q", n, w, got)
	}
	if !strings.HasSuffix(ansi.Strip(got), " ") {
		t.Errorf("selected row not padded to the column: %q", ansi.Strip(got))
	}
	// A title longer than the column is cut, never wrapped past it.
	r.e.pr.Title = strings.Repeat("x", 200)
	got = m.rowLine(r, true, m.prNumW(), w)
	if n := ansi.StringWidth(got); n != w {
		t.Errorf("long selected row width = %d, want %d", n, w)
	}
	// Unselected rows are truncated to the column too.
	got = m.rowLine(r, false, m.prNumW(), w)
	if n := ansi.StringWidth(got); n != w {
		t.Errorf("long unselected row width = %d, want %d", n, w)
	}
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

func TestMouseWheelScrollsPreviewFromEitherColumn(t *testing.T) {
	m := testModel(t)
	m.prevVP.SetContent(strings.Repeat("line\n", 100))
	first := m.cursor
	res, _ := m.Update(wheelDown(2)) // pointer over the list
	m = res.(model)
	if m.cursor != first || m.prevVP.YOffset() == 0 {
		t.Errorf("wheel over list: cursor=%d (was %d) preview=%d, want cursor unchanged and preview>0", m.cursor, first, m.prevVP.YOffset())
	}
	before := m.prevVP.YOffset()
	res, _ = m.Update(wheelDown(m.listW() + 10)) // pointer over the preview
	m = res.(model)
	if m.prevVP.YOffset() <= before || m.cursor != first {
		t.Errorf("wheel over preview: cursor=%d preview=%d, want cursor=%d preview>%d", m.cursor, m.prevVP.YOffset(), first, before)
	}
	res, _ = m.Update(wheelUp(2))
	after := res.(model)
	if after.prevVP.YOffset() >= m.prevVP.YOffset() {
		t.Errorf("wheel up did not scroll the preview back: %d -> %d", m.prevVP.YOffset(), after.prevVP.YOffset())
	}
}

func TestMouseWheelBurstNeverTypesIntoFilter(t *testing.T) {
	m := testModel(t)
	m.prevVP.SetContent(strings.Repeat("line\n", 100))
	first := m.cursor
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
	if got.cursor != first {
		t.Errorf("burst moved the selection: cursor=%d want %d", got.cursor, first)
	}
	if got.prevVP.YOffset() == 0 {
		t.Errorf("burst did not scroll the preview")
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

func TestMouseClickSelectsRowWithoutOpening(t *testing.T) {
	m := testModel(t)
	first := m.cursor
	// rows: header(alpha) #100 header(beta) #7 → the second PR sits on row 3,
	// which is screen line 4 (line 0 is the filter input).
	click := func(x, y int) tea.MouseClickMsg {
		return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
	}
	res, cmd := m.Update(click(3, 4))
	got := res.(model)
	if got.cursor == first || got.currentRow() == nil || got.currentRow().e.pr.Number != 7 {
		t.Fatalf("click did not select #7: cursor=%d", got.cursor)
	}
	_ = cmd
	if got.action != nil {
		t.Errorf("click queued an action: %v", got.action)
	}
	// Clicking a header or the preview column changes nothing.
	for _, c := range []tea.MouseClickMsg{click(3, 3), click(got.listW()+10, 2)} {
		res, _ = got.Update(c)
		if res.(model).cursor != got.cursor {
			t.Errorf("click %+v moved the cursor to %d", c, res.(model).cursor)
		}
	}
	// Right click is ignored too.
	res, _ = got.Update(tea.MouseClickMsg{X: 3, Y: 2, Button: tea.MouseRight})
	if res.(model).cursor != got.cursor {
		t.Errorf("right click moved the cursor")
	}
}

func TestCtrlOQueuesBrowserOpenAndQuits(t *testing.T) {
	m := testModel(t)
	res, cmd := m.handleKey(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	got := res.(model)
	if got.browse != "u1" {
		t.Errorf("browse=%q, want the selected PR URL u1", got.browse)
	}
	if got.action != nil {
		t.Errorf("ctrl+o queued a herdr action: %v", got.action)
	}
	if cmd == nil {
		t.Fatalf("ctrl+o did not quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+o cmd is not tea.Quit")
	}
	if got.ti.Value() != "" {
		t.Errorf("ctrl+o leaked into the filter: %q", got.ti.Value())
	}
}
