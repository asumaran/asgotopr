package main

// The bubbletea model: a filter input on top, a two-column body (grouped PR
// list left, markdown preview right) and a help footer. Modeled on
// asgoto: the input is focused before the program starts, every printable
// key filters, and the selected action is executed as a herdr CLI call AFTER
// the TUI exits (quitting is what closes the popup).

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func herdrBin() string {
	if b := os.Getenv("HERDR_BIN_PATH"); b != "" {
		return b
	}
	return "herdr"
}

func homeRel(p string) string {
	if h, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, h) {
		return "~" + strings.TrimPrefix(p, h)
	}
	return p
}

func truncate(s string, width int) string {
	return ansi.Truncate(s, width, "…")
}

// ---- styles ----

var (
	stPrompt  = lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true)
	stDev     = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
	stHeader  = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	stDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	stTitle   = lipgloss.NewStyle().Bold(true)
	stError   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	stKeyHint = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	stCount   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	stPROpen  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	stPRDraft = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	// preview header facts
	stFactKey    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	stOK         = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	stBad        = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	stWarn       = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	stBadgeDraft = lipgloss.NewStyle().Background(lipgloss.Color("3")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1)
	stBadgeBad   = lipgloss.NewStyle().Background(lipgloss.Color("1")).Foreground(lipgloss.Color("15")).Bold(true).Padding(0, 1)
)

// ---- key bindings ----

type keyMap struct {
	Nav      listNav
	Select   key.Binding
	Cancel   key.Binding
	PrevUp   key.Binding
	PrevDown key.Binding
	Shrink   key.Binding
	Grow     key.Binding
	Browse   key.Binding
	Filter   key.Binding
	Help     key.Binding
}

// ShortHelp is the folded help line: the tool's own actions, the help and the
// quit keys. Moving, scrolling and resizing are in the expanded help, so the
// line stays short enough for a narrow popup (a cut line loses the quit keys
// first).
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Filter, k.Select, k.Browse, k.Help, k.Cancel}
}

// FullHelp is what `?` expands the help into, one column per group: the
// filter and the preview, the list, the tool's actions, help and quit.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Filter, k.PrevUp, k.Shrink},
		{k.Nav.Up, k.Nav.PageUp, k.Nav.Top},
		{k.Select, k.Browse},
		{k.Help, k.Cancel},
	}
}

func defaultKeys() keyMap {
	return keyMap{
		Nav:      defaultListNav(),
		Select:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Cancel:   key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc/q", "quit")),
		PrevUp:   key.NewBinding(key.WithKeys("shift+up"), key.WithHelp("⇧↑/⇧↓", "scroll the description")),
		PrevDown: key.NewBinding(key.WithKeys("shift+down")),
		Shrink:   key.NewBinding(key.WithKeys("shift+left"), key.WithHelp("⇧←/⇧→", "resize the list")),
		Grow:     key.NewBinding(key.WithKeys("shift+right")),
		Browse:   key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("^o", "browser")),
		// Help-only entry: a binding without keys is disabled and the help
		// bubble would skip it. Nothing ever matches against it.
		Filter: key.NewBinding(key.WithKeys("type"), key.WithHelp("type", "filter")),
		Help:   helpKey,
	}
}

// ---- model ----

type uiMode int

const (
	modeFilter uiMode = iota
	modeConfirmStash
	modeBusy
	modeError
)

type model struct {
	// data
	entries []*entry
	titles  []string // parallel corpora, see filter.go
	branchC []string
	metas   []string
	slugs   map[string][]localRepo
	cache   prCache

	// background refresh
	pendingSearches int
	searchResults   [][]prItem
	searchErr       string
	mergedPending   []prItem // fresh list waiting for its bodiesMsg
	refreshing      bool
	netErr          string

	// ui
	rows    []row
	cursor  int
	mode    uiMode
	pending *entry // target of the confirm/busy flow
	busyMsg string
	errMsg  string
	ti      textinput.Model
	listVP  viewport.Model
	prevVP  viewport.Model
	help    help.Model
	keys    keyMap
	width   int
	height  int
	split   int // the preview's share of the width, percent

	// preview render cache
	renders map[string]string
	prevKey string

	// previewStyle is the glamour standard style ("dark"/"light"). It starts
	// as "dark" and flips when the terminal answers RequestBackgroundColor.
	previewStyle string

	action []string // herdr CLI args to run after quit (nil = none)
	browse string   // PR URL to open in the browser after quit ("" = none)
}

func (m *model) currentRow() *row {
	if m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].kind == "pr" {
		return &m.rows[m.cursor]
	}
	return nil
}

// innerW is the width inside the frame's sides.
func (m *model) innerW() int { return max(20, m.width-2) }

// listW is the list's share of the main section; the divider and the preview
// take the rest.
func (m *model) listW() int { w, _ := splitWidths(m.innerW(), m.split); return w }

// detailsW is the preview's area, including the cell of padding on each
// side; prevW is the text width inside it.
func (m *model) detailsW() int { _, w := splitWidths(m.innerW(), m.split); return w }
func (m *model) prevW() int    { return max(10, m.detailsW()-2) }

// bodyH is the height of the main section: everything but the frame's own
// lines and the help.
func (m *model) bodyH() int { return max(1, m.height-frameRows(false)-m.footH()) }

// footH is the height of the foot: a message takes one line, the help more
// while `?` has it expanded; the main section keeps at least minBodyH.
func (m *model) footH() int {
	if m.footMsg() != "" {
		return 1
	}
	return helpHeight(m.help, m.keys, m.height-frameRows(false)-minBodyH)
}

const minBodyH = 4

func (m *model) toggleHelp() tea.Cmd {
	m.help.ShowAll = !m.help.ShowAll
	m.resize()
	m.renderList()
	return m.updatePreview()
}

func (m *model) resize() {
	m.listVP.SetWidth(m.listW())
	m.listVP.SetHeight(m.bodyH())
	m.prevVP.SetWidth(m.prevW())
	m.syncPreviewHeight()
	m.help.SetWidth(max(0, m.width-4))
}

// resizeList moves the divider between the list and the preview by one step.
func (m *model) resizeList(grow bool) tea.Cmd {
	m.split = stepSplit(m.split, grow)
	saveSplit(stateDir(), m.split)
	m.resize()
	m.renderList()
	return m.updatePreview()
}

// syncPreviewHeight fits the body viewport under the header of the selected
// PR. The header height varies (labels wrap, failed checks add a line) and
// changes on refresh without touching the preview key, so this runs on every
// selection change and resize rather than being cached.
func (m *model) syncPreviewHeight() {
	hh := 0
	if r := m.currentRow(); r != nil {
		hh = lipgloss.Height(previewHeader(r.e.pr, m.prevW()))
	}
	prevH := m.bodyH() - hh
	if prevH < 1 {
		prevH = 1
	}
	m.prevVP.SetHeight(prevH)
}

func (m *model) setEntries(prs []prItem) {
	m.entries = buildEntries(prs, m.slugs)
	m.titles, m.branchC, m.metas = corpora(m.entries)
}

func (m *model) applyFilter() {
	q := strings.ToLower(m.ti.Value())
	m.rows = buildRows(m.entries, q, m.titles, m.branchC, m.metas)
	if q != "" {
		m.cursor = firstPR(m.rows) // ranked: the best match is the first row
		return
	}
	if m.cursor < 0 || m.cursor >= len(m.rows) || m.rows[m.cursor].kind != "pr" {
		m.cursor = firstPR(m.rows)
	}
}

func (m *model) keepCursorOn(url string) {
	if url == "" {
		return
	}
	for i, r := range m.rows {
		if r.kind == "pr" && r.e.pr.URL == url {
			m.cursor = i
			return
		}
	}
	m.cursor = firstPR(m.rows)
}

// ---- list rendering ----

// prNumW is the shared "#123" column width, over all entries so filtering
// doesn't shift columns.
func (m *model) prNumW() int {
	w := 0
	for _, e := range m.entries {
		if l := len("#" + strconv.Itoa(e.pr.Number)); l > w {
			w = l
		}
	}
	return w
}

func (m *model) renderList() {
	numW := m.prNumW()
	listW := m.listW()
	var b strings.Builder
	for i, r := range m.rows {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(m.rowLine(r, i == m.cursor, numW, listW))
	}
	m.listVP.SetContent(b.String())
	m.ensureVisible()
}

// rowLine renders one list row truncated to width. The selected row is
// padded to the full width before styling so its background spans the whole
// column instead of stopping at the end of the title.
func (m *model) rowLine(r row, selected bool, numW, width int) string {
	if r.kind == "header" {
		return truncate(stHeader.Render(r.repo.Name), width)
	}
	num := "#" + strconv.Itoa(r.e.pr.Number)
	pad := strings.Repeat(" ", numW-len(num))
	if selected {
		return selPad(truncate(stSel.Render("▌ "+num+pad+" ")+highlight(r.e.pr.Title, r.idx, stSel), width), width)
	}
	numStyle := stPROpen
	if r.e.pr.IsDraft {
		numStyle = stPRDraft
	}
	title := r.e.pr.Title
	if r.match && len(r.idx) > 0 {
		title = highlight(title, r.idx, lipgloss.NewStyle())
	}
	return truncate("  "+numStyle.Render(num)+pad+" "+title, width)
}

func (m *model) ensureVisible() {
	h := m.listVP.Height()
	if h <= 0 || m.cursor < 0 {
		return
	}
	// Scrolling up onto the first PR of a group also reveals its header, so
	// the repo name never sits hidden one line above the selection.
	top := m.cursor
	if top > 0 && m.rows[top-1].kind == "header" {
		top--
	}
	if top < m.listVP.YOffset() {
		m.listVP.SetYOffset(top)
	} else if m.cursor >= m.listVP.YOffset()+h {
		m.listVP.SetYOffset(m.cursor - h + 1)
	}
}

// ---- preview ----

func (m *model) updatePreview() tea.Cmd {
	m.syncPreviewHeight()
	r := m.currentRow()
	if r == nil {
		m.prevKey = ""
		m.prevVP.SetContent("")
		return nil
	}
	key := previewKey(r.e.pr, m.prevW())
	if key == m.prevKey {
		return nil
	}
	m.prevKey = key
	m.prevVP.GotoTop()
	if c, ok := m.renders[key]; ok {
		m.prevVP.SetContent(c)
		return nil
	}
	m.prevVP.SetContent(stDim.Render("rendering…"))
	return renderPreviewCmd(r.e.pr, m.prevW(), m.previewStyle)
}

// setPreviewStyle switches the glamour style once the terminal background is
// known. Cached renders carry the old palette, so they are dropped and the
// current preview is rendered again.
func (m *model) setPreviewStyle(style string) tea.Cmd {
	if style == m.previewStyle {
		return nil
	}
	m.previewStyle = style
	m.renders = map[string]string{}
	m.prevKey = ""
	return m.updatePreview()
}

// ---- refresh plumbing ----

// finishRefresh swaps in a freshly fetched PR list. updateCache is false when
// bodies failed to fetch: persisting then would freeze empty bodies until the
// next updatedAt bump.
func (m *model) finishRefresh(prs []prItem, updateCache bool) tea.Cmd {
	m.refreshing = false
	if updateCache {
		m.cache = prCache{FetchedAt: time.Now(), PRs: prs}
		savePRCache(m.cache)
	}
	var curURL string
	if r := m.currentRow(); r != nil {
		curURL = r.e.pr.URL
	}
	m.setEntries(prs)
	m.applyFilter()
	m.keepCursorOn(curURL)
	m.renderList()
	return m.updatePreview()
}

// ---- selection ----

func (m *model) handleSelect() tea.Cmd {
	r := m.currentRow()
	if r == nil {
		return nil
	}
	e := r.e
	// A worktree (or the main checkout) already on the PR's branch wins, in
	// whichever clone of this origin has it. `herdr worktree open` is
	// idempotent: it adds the repo/worktree to the sidebar when missing and
	// focuses it either way.
	for _, clone := range m.slugs[e.pr.RepoSlug] {
		if wt := worktreeForBranch(clone.Path, e.pr.HeadRefName); wt != nil {
			m.action = []string{"worktree", "open", "--cwd", clone.Path, "--path", wt.Path, "--focus"}
			return tea.Quit
		}
	}
	// Nothing has the branch: switch the primary clone.
	m.pending = e
	if isDirty(e.repo.Path) {
		m.mode = modeConfirmStash
		return nil
	}
	return m.startSwitch()
}

func (m *model) startSwitch() tea.Cmd {
	m.mode = modeBusy
	m.busyMsg = "Switching to " + m.pending.pr.HeadRefName + "…"
	return performSwitchCmd(m.pending)
}

// ---- bubbletea ----

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink, tea.RequestBackgroundColor}
	if m.refreshing {
		cmds = append(cmds, fetchSearchCmd("author"), fetchSearchCmd("assignee"))
	}
	if c := m.updatePreview(); c != nil {
		cmds = append(cmds, c)
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		m.renderList()
		return m, m.updatePreview()

	case tea.BackgroundColorMsg:
		style := "dark"
		if !msg.IsDark() {
			style = "light"
		}
		return m, m.setPreviewStyle(style)

	case searchMsg:
		m.pendingSearches--
		if msg.err != nil {
			m.searchErr = msg.err.Error()
		} else {
			m.searchResults = append(m.searchResults, msg.prs)
		}
		if m.pendingSearches > 0 {
			return m, nil
		}
		results := m.searchResults
		m.searchResults = nil
		if m.searchErr != "" { // keep the cached view on any failure
			m.refreshing = false
			m.netErr = m.searchErr
			m.searchErr = ""
			return m, nil
		}
		merged, stale := planRefresh(m.cache.byURL(), mergePRs(results...))
		if len(stale) == 0 {
			return m, m.finishRefresh(merged, true)
		}
		m.mergedPending = merged
		return m, fetchBodiesCmd(stale)

	case bodiesMsg:
		merged := m.mergedPending
		m.mergedPending = nil
		if msg.err != nil {
			m.netErr = msg.err.Error()
			return m, m.finishRefresh(merged, false)
		}
		return m, m.finishRefresh(applyBodies(merged, msg.bodies), true)

	case previewMsg:
		if msg.style != m.previewStyle { // rendered before the style flipped
			return m, nil
		}
		if m.renders == nil {
			m.renders = map[string]string{}
		}
		m.renders[msg.key] = msg.content
		if msg.key == m.prevKey {
			m.prevVP.SetContent(msg.content)
		}
		return m, nil

	case stashedMsg:
		if msg.err != nil {
			m.mode = modeError
			m.errMsg = msg.err.Error()
			return m, nil
		}
		return m, m.startSwitch()

	case switchedMsg:
		if msg.err != nil {
			m.mode = modeError
			m.errMsg = msg.err.Error()
			return m, nil
		}
		repo := m.pending.repo
		m.action = []string{"worktree", "open", "--cwd", repo.Path, "--path", repo.Path, "--focus"}
		return m, tea.Quit

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.MouseWheelMsg:
		return m.handleMouse(msg)

	case tea.MouseClickMsg:
		return m.handleClick(msg)

	default:
		var cmd tea.Cmd
		m.ti, cmd = m.ti.Update(msg)
		return m, cmd
	}
}

func (m model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeConfirmStash:
		switch msg.String() {
		case "s", "enter":
			m.mode = modeBusy
			m.busyMsg = "Stashing changes…"
			return m, stashCmd(m.pending.repo, m.pending.pr.HeadRefName)
		case "f":
			return m, m.startSwitch()
		case "esc", "ctrl+c":
			m.mode = modeFilter
			m.pending = nil
		}
		return m, nil

	case modeError:
		switch msg.String() {
		case "esc", "enter", "ctrl+c":
			m.mode = modeFilter
			m.pending = nil
			m.errMsg = ""
		}
		return m, nil

	case modeBusy:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}

	// modeFilter
	switch {
	case foldsHelp(msg, m.help):
		return m, m.toggleHelp() // esc folds the help before it quits
	case isHelpKey(msg, m.ti.Value()):
		return m, m.toggleHelp()
	case msg.String() == "q" && m.ti.Value() == "":
		// q quits only while the filter is empty; otherwise it is text.
		return m, tea.Quit
	case key.Matches(msg, m.keys.Cancel):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Select):
		return m, m.handleSelect()
	case key.Matches(msg, m.keys.Browse):
		if r := m.currentRow(); r != nil {
			m.browse = r.e.pr.URL
			return m, tea.Quit
		}
		return m, nil
	case m.keys.Nav.matches(msg):
		m.cursor = m.keys.Nav.move(msg, m.cursor, len(m.rows), m.listVP.Height(),
			func(i int) bool { return m.rows[i].kind == "pr" })
		m.renderList()
		return m, m.updatePreview()
	case key.Matches(msg, m.keys.Shrink):
		return m, m.resizeList(false)
	case key.Matches(msg, m.keys.Grow):
		return m, m.resizeList(true)
	case key.Matches(msg, m.keys.PrevUp):
		m.prevVP.ScrollUp(3)
		return m, nil
	case key.Matches(msg, m.keys.PrevDown):
		m.prevVP.ScrollDown(3)
		return m, nil
	}

	var curURL string
	if r := m.currentRow(); r != nil {
		curURL = r.e.pr.URL
	}
	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	m.applyFilter()
	if m.ti.Value() == "" {
		// Clearing the query rebuilt the rows; stay on the same PR instead of
		// whatever now sits at the old cursor index.
		m.keepCursorOn(curURL)
	}
	m.renderList()
	return m, tea.Batch(cmd, m.updatePreview())
}

// handleMouse routes the wheel by the pointer, as asgitlog does: over the
// list it moves the selection, anywhere else it scrolls the description.
// Sending every event to the preview was the rule for a while, because SGR
// mouse reports carry no gesture phase and trackpad inertia that drifts from
// one column to the other cannot be told apart from a new gesture; a list the
// wheel does nothing on turned out to be the worse of the two. Outside
// modeFilter the wheel is ignored (and the view stops requesting mouse reports
// at all).
func (m model) handleMouse(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeFilter {
		return m, nil
	}
	if m.overList(msg.X, msg.Y) {
		switch msg.Button {
		case tea.MouseWheelUp:
			return m.handleKey(tea.KeyPressMsg{Code: tea.KeyUp})
		case tea.MouseWheelDown:
			return m.handleKey(tea.KeyPressMsg{Code: tea.KeyDown})
		}
		return m, nil
	}
	m.prevVP, _ = m.prevVP.Update(msg)
	return m, nil
}

// overList reports whether a screen cell is inside the list.
func (m *model) overList(x, y int) bool {
	return m.mode == modeFilter && x >= 1 && x <= m.listW() && y >= listY(false) && y < listY(false)+m.bodyH()
}

// handleClick moves the selection to the PR row under a left click on the
// list. It never opens the PR: that stays on enter, so a stray click cannot
// switch branches. The list starts on screen row listY(false), inside the frame's
// left side, and is offset by its scroll position.
func (m model) handleClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft || !m.overList(msg.X, msg.Y) {
		return m, nil
	}
	i := msg.Y - listY(false) + m.listVP.YOffset()
	if i < 0 || i >= len(m.rows) || m.rows[i].kind != "pr" || i == m.cursor {
		return m, nil
	}
	m.cursor = i
	m.renderList()
	return m, m.updatePreview()
}

func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	// Mouse reports are only wanted while the two columns are scrollable;
	// the confirm/busy/error dialogs turn them off.
	if m.mode == modeFilter {
		v.MouseMode = tea.MouseModeCellMotion
	} else {
		v.MouseMode = tea.MouseModeNone
	}
	return v
}

// render stacks the sections in one frame (see frame.go). There is no context
// line: nothing here needs one.
func (m model) render() string {
	w := m.width
	out := frameHead(w, "", m.counter(), m.ti.View())
	pos := ""
	if m.mode == modeFilter && m.currentRow() != nil {
		pos = scrollPos(&m.prevVP)
	}
	out = append(out, splitMain(m.listLines(), strings.Split(m.rightColumn(), "\n"),
		m.listW(), m.detailsW(), listPos(&m.listVP, func(i int) bool { return i < len(m.rows) && m.rows[i].kind != "header" }), pos)...)
	for _, l := range m.footLines() {
		out = append(out, framed(w, l))
	}
	out = append(out, hline(w, "╰", "╯", "", ""))
	return strings.Join(out, "\n")
}

// counter is the matches/total count, with the refresh mark.
func (m model) counter() string {
	n := 0
	for _, r := range m.rows {
		if r.kind == "pr" {
			n++
		}
	}
	s := stCount.Render(strconv.Itoa(n) + "/" + strconv.Itoa(len(m.entries)))
	if m.refreshing {
		s += stDim.Render(" refreshing…")
	}
	return s
}

// listLines is the list as exactly bodyH lines of listW cells.
func (m model) listLines() []string {
	lines := strings.Split(m.listVP.View(), "\n")
	for len(lines) < m.bodyH() {
		lines = append(lines, "")
	}
	lines = lines[:m.bodyH()]
	for i, l := range lines {
		lines[i] = fit(l, m.listW())
	}
	return lines
}

func (m model) rightColumn() string {
	w := m.prevW()
	switch m.mode {
	case modeConfirmStash:
		return confirmView(m.pending, w)
	case modeError:
		return errorView(m.errMsg, w)
	case modeBusy:
		return busyView(m.busyMsg, w)
	}
	r := m.currentRow()
	if r == nil {
		if len(m.entries) == 0 && !m.refreshing {
			return "\n" + stDim.Render("No open PRs")
		}
		return ""
	}
	return previewHeader(r.e.pr, w) + "\n" + m.prevVP.View()
}

// footer is the key help, or the network error while there is one. The
// refresh mark lives next to the counter.
// footMsg is what takes the help's place while there is something to say.
func (m model) footMsg() string {
	if m.netErr != "" {
		return stError.Render(truncate(m.netErr, max(0, m.width-4)))
	}
	return ""
}

func (m model) footLines() []string {
	if msg := m.footMsg(); msg != "" {
		return []string{msg}
	}
	return helpLines(m.help, m.keys, m.width-4, m.footH())
}

// promptText builds the textinput prompt, with an orange "(dev)" marker on
// non-release builds.
func promptText() string {
	if strings.HasPrefix(version, "v") {
		return stPrompt.Render("asgotopr ❯ ")
	}
	return stPrompt.Render("asgotopr (") + stDev.Render("dev") + stPrompt.Render(") ❯ ")
}

// openURL opens a PR in the browser. ASGOTOPR_OPENER, when set, is used as-is
// (the pty driver points it at a logging stub). Otherwise, when Google Chrome
// is running with a window, the tab is created in Chrome's front window so it
// lands in the profile the user last focused: plain `open` hands the URL to
// Chrome, which then picks its own "last used" profile bookkeeping, and that
// routinely disagrees with the window you were just looking at. Anything else
// falls back to `open`. Outside macOS it is xdg-open.
func openURL(url string) {
	if b := os.Getenv("ASGOTOPR_OPENER"); b != "" {
		_ = exec.Command(b, url).Run()
		return
	}
	if runtime.GOOS != "darwin" {
		_ = exec.Command("xdg-open", url).Run()
		return
	}
	if openInChromeFrontWindow(url) == nil {
		return
	}
	_ = exec.Command("open", url).Run()
}

func openInChromeFrontWindow(url string) error {
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(url)
	script := `tell application "Google Chrome"
	if not running then error "not running"
	if (count of windows) = 0 then error "no windows"
	tell front window to make new tab with properties {URL:"` + esc + `"}
	activate
end tell`
	return exec.Command("osascript", "-e", script).Run()
}

// runAction executes the queued post-quit work: the herdr CLI call and/or
// the browser open. Both wait for the TUI to exit because quitting is what
// closes the popup and anything written to the terminal after that is lost.
func runAction(action []string, browse string) {
	if action != nil {
		_ = exec.Command(herdrBin(), action...).Run()
	}
	if browse != "" {
		openURL(browse)
	}
}
