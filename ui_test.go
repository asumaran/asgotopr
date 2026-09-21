package main

import (
	"os"
	"path/filepath"
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
		split:   splitDefault,
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

// TestMouseWheelFollowsThePointer: over the list the wheel moves the
// selection, as in asgitlog; anywhere else it scrolls the preview.
func TestMouseWheelFollowsThePointer(t *testing.T) {
	m := testModel(t)
	first := m.cursor
	res, _ := m.Update(wheelDown(2)) // pointer over the list
	m = res.(model)
	if m.cursor <= first || m.rows[m.cursor].kind != "pr" {
		t.Errorf("wheel down over the list: cursor=%d (was %d), want the next PR", m.cursor, first)
	}
	res, _ = m.Update(wheelUp(2))
	m = res.(model)
	if m.cursor != first {
		t.Errorf("wheel up over the list: cursor=%d, want %d", m.cursor, first)
	}

	m.prevVP.SetContent(strings.Repeat("line\n", 100))
	res, _ = m.Update(wheelDown(m.listW() + 10)) // pointer over the preview
	m = res.(model)
	if m.prevVP.YOffset() == 0 || m.cursor != first {
		t.Errorf("wheel over the preview: cursor=%d preview=%d, want cursor=%d preview>0", m.cursor, m.prevVP.YOffset(), first)
	}
	res, _ = m.Update(wheelUp(m.listW() + 10))
	if after := res.(model); after.prevVP.YOffset() >= m.prevVP.YOffset() {
		t.Errorf("wheel up did not scroll the preview back: %d -> %d", m.prevVP.YOffset(), after.prevVP.YOffset())
	}
}

func TestMouseWheelBurstNeverTypesIntoFilter(t *testing.T) {
	m := testModel(t)
	m.prevVP.SetContent(strings.Repeat("line\n", 100))
	first := m.cursor
	var mm tea.Model = m
	for i := 0; i < 200; i++ {
		mm, _ = mm.Update(wheelDown(m.listW() + 10))
	}
	got := mm.(model)
	if got.ti.Value() != "" {
		t.Errorf("filter got mouse text %q", got.ti.Value())
	}
	if got.cursor != first {
		t.Errorf("a burst over the preview moved the selection: cursor=%d want %d", got.cursor, first)
	}
	if got.prevVP.YOffset() == 0 {
		t.Errorf("burst did not scroll the preview")
	}
	for i := 0; i < 200; i++ { // and over the list: it stops on the last PR
		mm, _ = mm.Update(wheelDown(2))
	}
	if got = mm.(model); got.ti.Value() != "" || got.rows[got.cursor].kind != "pr" {
		t.Errorf("burst over the list: filter=%q cursor on %q", got.ti.Value(), got.rows[got.cursor].kind)
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
	// which is screen line listY(false)+3 (the list starts at listY(false), inside the frame).
	click := func(x, y int) tea.MouseClickMsg {
		return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
	}
	res, cmd := m.Update(click(3, listY(false)+3))
	got := res.(model)
	if got.cursor == first || got.currentRow() == nil || got.currentRow().e.pr.Number != 7 {
		t.Fatalf("click did not select #7: cursor=%d", got.cursor)
	}
	_ = cmd
	if got.action != nil {
		t.Errorf("click queued an action: %v", got.action)
	}
	// Clicking a header, the preview, the divider or the frame changes nothing.
	for _, c := range []tea.MouseClickMsg{click(3, listY(false)+2), click(got.listW()+10, listY(false)+1),
		click(got.listW()+1, listY(false)+1), click(0, listY(false)+1), click(3, mainY(false)), click(3, 1)} {
		res, _ = got.Update(c)
		if res.(model).cursor != got.cursor {
			t.Errorf("click %+v moved the cursor to %d", c, res.(model).cursor)
		}
	}
	// Right click is ignored too.
	res, _ = got.Update(tea.MouseClickMsg{X: 3, Y: listY(false) + 1, Button: tea.MouseRight})
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

// TestFrameGeometry pins the single-frame layout: exactly height lines, each
// exactly width cells, sections where the click math expects them.
func TestFrameGeometry(t *testing.T) {
	m := testModel(t)
	lines := strings.Split(m.View().Content, "\n")
	if len(lines) != m.height {
		t.Errorf("%d lines, want %d", len(lines), m.height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("line %d is %d cells, want %d: %q", i, w, m.width, ansi.Strip(l))
		}
	}
	plain := strings.Split(ansi.Strip(m.View().Content), "\n")
	if !strings.HasPrefix(plain[0], "╭") || !strings.HasPrefix(plain[len(plain)-1], "╰") ||
		!strings.Contains(plain[mainY(false)], "┬") || !strings.Contains(plain[len(plain)-3], "─ 2/2 ─┴") {
		t.Errorf("frame sections misplaced:\n%s", strings.Join(plain, "\n"))
	}
	if !strings.Contains(plain[len(plain)-2], "type filter") || !strings.Contains(plain[len(plain)-2], "esc/q quit") {
		t.Errorf("help line = %q", plain[len(plain)-2])
	}
}

func TestQQuitsOnlyWithEmptyFilter(t *testing.T) {
	_, cmd := testModel(t).handleKey(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("q with an empty filter should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q cmd is not tea.Quit")
	}
	res, _ := testModel(t).handleKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
	res, _ = res.(model).handleKey(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if got := res.(model).ti.Value(); got != "xq" {
		t.Errorf("filter = %q, want q typed as text", got)
	}
}

// TestMain sandboxes the state dir: tests must never touch the real one.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "asgotopr-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HERDR_PLUGIN_STATE_DIR", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// TestResizeList: shift+arrows move the divider, the frame still fits, and
// the position is there for the next run.
func TestResizeList(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_STATE_DIR", t.TempDir())
	next, _ := testModel(t).Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m := next.(model)
	m.split = splitDefault
	m.resize()
	w := m.listW()
	shift := func(code rune) {
		next, _ := m.Update(tea.KeyPressMsg{Code: code, Mod: tea.ModShift})
		m = next.(model)
	}

	shift(tea.KeyRight)
	if m.listW() <= w || m.split != splitDefault-splitStep || loadSplit(stateDir()) != m.split {
		t.Errorf("grow: list %d -> %d, split=%d, saved=%d", w, m.listW(), m.split, loadSplit(stateDir()))
	}
	if m.listVP.Width() != m.listW() || m.prevVP.Width() != m.prevW() {
		t.Errorf("viewports %d | %d, want %d | %d", m.listVP.Width(), m.prevVP.Width(), m.listW(), m.prevW())
	}
	for i, l := range strings.Split(m.View().Content, "\n") {
		if got := ansi.StringWidth(l); got != m.width {
			t.Errorf("line %d is %d cells after the resize, want %d", i, got, m.width)
		}
	}

	shift(tea.KeyLeft)
	shift(tea.KeyLeft)
	if m.listW() >= w || m.split != splitDefault+splitStep {
		t.Errorf("shrink: list %d -> %d, split=%d", w, m.listW(), m.split)
	}
	for range 10 {
		shift(tea.KeyLeft)
	}
	if m.split != splitMax {
		t.Errorf("split should clamp at %d, got %d", splitMax, m.split)
	}
}

// TestCopyKeyCopiesTheURL covers ctrl+y: the PR under the cursor goes to the
// clipboard, the help line confirms it for a moment, and the filter is left
// alone.
func TestCopyKeyCopiesTheURL(t *testing.T) {
	log := filepath.Join(t.TempDir(), "clip")
	stub := filepath.Join(t.TempDir(), "clipboard")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\ncat > "+log+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ASGOTOPR_CLIPBOARD", stub)
	m := testModel(t)
	res, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+y returned no command")
	}
	res, _ = res.(model).Update(cmd())
	m = res.(model)
	if got, _ := os.ReadFile(log); string(got) != "u1" {
		t.Errorf("the clipboard got %q, want the URL of the PR under the cursor", got)
	}
	plain := strings.Split(ansi.Strip(m.View().Content), "\n")
	if help := plain[len(plain)-2]; !strings.Contains(help, "copied u1") {
		t.Errorf("help line = %q, want the confirmation", help)
	}
	if m.ti.Value() != "" {
		t.Errorf("ctrl+y leaked into the filter: %q", m.ti.Value())
	}
	res, _ = m.Update(clearFlashMsg(m.flash.seq))
	plain = strings.Split(ansi.Strip(res.(model).View().Content), "\n")
	if help := plain[len(plain)-2]; !strings.Contains(help, "type filter") {
		t.Errorf("after the timer the help is back: %q", help)
	}
}

// TestPanel: f1 lays the keys over a frame that keeps its size, takes
// every key while it is open, and esc closes it before it quits. `?` is text
// for the filter. This tool has no options, so the panel lists the keys alone.
func TestPanel(t *testing.T) {
	m := testModel(t)
	press := func(keys ...tea.KeyPressMsg) {
		for _, k := range keys {
			res, _ := m.Update(k)
			m = res.(model)
		}
	}
	closed := strings.Split(ansi.Strip(m.render()), "\n")
	if foot := closed[len(closed)-2]; !strings.Contains(foot, "f1 help") {
		t.Fatalf("the help line offers the panel: %q", foot)
	}
	list := m.listVP.Height()
	press(tea.KeyPressMsg{Code: tea.KeyF1})
	open := strings.Split(ansi.Strip(m.render()), "\n")
	if len(open) != len(closed) || m.listVP.Height() != list {
		t.Fatalf("the panel changed the frame: %d lines (list %d), want %d (list %d)", len(open), m.listVP.Height(), len(closed), list)
	}
	all := strings.Join(open, "\n")
	if strings.Contains(all, "Options") {
		t.Errorf("no options here, so no such section:\n%s", all)
	}
	for _, want := range []string{"╭─ help ", "Keys", "esc close", "copy the URL", "scroll the description", "resize the list"} {
		if !strings.Contains(all, want) {
			t.Errorf("the panel lacks %q:\n%s", want, all)
		}
	}
	for i, l := range open {
		if ansi.StringWidth(l) != m.width {
			t.Errorf("line %d is %d cells wide, want %d", i, ansi.StringWidth(l), m.width)
		}
	}
	cursor := m.cursor
	press(tea.KeyPressMsg{Code: 'z', Text: "z"}, tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.ti.Value() != "" || m.cursor != cursor || !m.panel.open {
		t.Errorf("the panel should take every key: filter %q, cursor %d -> %d, open %v", m.ti.Value(), cursor, m.cursor, m.panel.open)
	}
	res, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = res.(model)
	if m.panel.open || cmd != nil {
		t.Errorf("esc closes the panel and nothing else: open=%v cmd=%v", m.panel.open, cmd)
	}
	press(tea.KeyPressMsg{Code: 'x', Text: "x"}, tea.KeyPressMsg{Code: '?', Text: "?"})
	if m.ti.Value() != "x?" || m.panel.open {
		t.Errorf("? is text: filter %q, panel open %v", m.ti.Value(), m.panel.open)
	}
	press(tea.KeyPressMsg{Code: tea.KeyF1})
	if !m.panel.open {
		t.Errorf("f1 opens the panel whatever the filter says")
	}
}

// TestEmptyListSaysWhy: a query that matches nothing says so in the list, as
// in every tool of the family (emptyList in listnav.go).
func TestEmptyListSaysWhy(t *testing.T) {
	m := testModel(t)
	for _, r := range "zzzzqq" {
		res, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = res.(model)
	}
	if len(m.rows) != 0 {
		t.Fatalf("the query should match nothing, got %d rows", len(m.rows))
	}
	list := ansi.Strip(m.listLines()[0])
	if !strings.HasPrefix(list, " No matches") {
		t.Errorf("the list should say there are no matches: %q", list)
	}
}
