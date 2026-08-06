package main

// The bubbletea model: a filter input on top, a two-column body (grouped PR
// list left, markdown preview right) and a help footer. Modeled on
// herdr-goto: the input is focused before the program starts, every printable
// key filters, and the selected action is executed as a herdr CLI call AFTER
// the TUI exits (quitting is what closes the popup).

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	stSel     = lipgloss.NewStyle().Background(lipgloss.Color("8")).Bold(true)
	stMatch   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	stHeader  = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	stDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	stTitle   = lipgloss.NewStyle().Bold(true)
	stError   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	stKeyHint = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	stPROpen  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	stPRDraft = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// ---- key bindings ----

type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Select   key.Binding
	Cancel   key.Binding
	PrevUp   key.Binding
	PrevDown key.Binding
	Filter   key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Filter, k.Up, k.Down, k.Select, k.PrevDown, k.Cancel}
}
func (k keyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

func defaultKeys() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑/^p", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "ctrl+n"), key.WithHelp("↓/^n", "down")),
		Select:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Cancel:   key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "cancel")),
		PrevUp:   key.NewBinding(key.WithKeys("shift+up", "pgup"), key.WithHelp("⇧↑", "")),
		PrevDown: key.NewBinding(key.WithKeys("shift+down", "pgdown"), key.WithHelp("⇧↓", "scroll desc")),
		Filter:   key.NewBinding(key.WithKeys(), key.WithHelp("type", "search")),
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

	// preview render cache
	renders map[string]string
	prevKey string

	action []string // herdr CLI args to run after quit (nil = none)
}

func (m *model) currentRow() *row {
	if m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].kind == "pr" {
		return &m.rows[m.cursor]
	}
	return nil
}

func (m *model) listW() int {
	w := (m.width - 3) * 30 / 100
	if w < 20 {
		w = 20
	}
	return w
}

func (m *model) prevW() int {
	w := m.width - m.listW() - 3
	if w < 10 {
		w = 10
	}
	return w
}

func (m *model) bodyH() int {
	h := m.height - 2 // input + footer
	if h < 1 {
		h = 1
	}
	return h
}

func (m *model) resize() {
	m.listVP.Width = m.listW()
	m.listVP.Height = m.bodyH()
	m.prevVP.Width = m.prevW()
	m.prevVP.Height = m.bodyH() - 3 // preview header (2 lines) + blank
	if m.prevVP.Height < 1 {
		m.prevVP.Height = 1
	}
	m.help.Width = m.width
}

func (m *model) setEntries(prs []prItem) {
	m.entries = buildEntries(prs, m.slugs)
	m.titles, m.branchC, m.metas = corpora(m.entries)
}

func (m *model) applyFilter() {
	q := strings.ToLower(m.ti.Value())
	m.rows = buildRows(m.entries, q, m.titles, m.branchC, m.metas)
	if q != "" {
		if b := bestMatch(m.rows); b >= 0 {
			m.cursor = b
		} else {
			m.cursor = firstPR(m.rows)
		}
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
		b.WriteString(truncate(m.rowLine(r, i == m.cursor, numW), listW))
	}
	m.listVP.SetContent(b.String())
	m.ensureVisible()
}

func (m *model) rowLine(r row, selected bool, numW int) string {
	if r.kind == "header" {
		return stHeader.Render(r.repo.Name)
	}
	num := "#" + strconv.Itoa(r.e.pr.Number)
	pad := strings.Repeat(" ", numW-len(num))
	if selected {
		return stSel.Render("▌ " + num + pad + " " + r.e.pr.Title)
	}
	numStyle := stPROpen
	if r.e.pr.IsDraft {
		numStyle = stPRDraft
	}
	title := r.e.pr.Title
	if r.match && len(r.idx) > 0 {
		title = highlight(title, r.idx)
	}
	return "  " + numStyle.Render(num) + pad + " " + title
}

// highlight styles the fuzzy-matched characters within a label.
func highlight(label string, idx []int) string {
	set := make(map[int]bool, len(idx))
	for _, i := range idx {
		set[i] = true
	}
	var b strings.Builder
	for i, r := range []rune(label) {
		if set[i] {
			b.WriteString(stMatch.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (m *model) ensureVisible() {
	h := m.listVP.Height
	if h <= 0 || m.cursor < 0 {
		return
	}
	if m.cursor < m.listVP.YOffset {
		m.listVP.SetYOffset(m.cursor)
	} else if m.cursor >= m.listVP.YOffset+h {
		m.listVP.SetYOffset(m.cursor - h + 1)
	}
}

// ---- preview ----

func (m *model) updatePreview() tea.Cmd {
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
	return renderPreviewCmd(r.e.pr, m.prevW())
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
	cmds := []tea.Cmd{textinput.Blink}
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

	case tea.KeyMsg:
		return m.handleKey(msg)

	default:
		var cmd tea.Cmd
		m.ti, cmd = m.ti.Update(msg)
		return m, cmd
	}
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	case key.Matches(msg, m.keys.Cancel):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Select):
		return m, m.handleSelect()
	case key.Matches(msg, m.keys.Up):
		m.cursor = nextPR(m.rows, m.cursor, -1)
		m.renderList()
		return m, m.updatePreview()
	case key.Matches(msg, m.keys.Down):
		m.cursor = nextPR(m.rows, m.cursor, +1)
		m.renderList()
		return m, m.updatePreview()
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

func (m model) View() string {
	right := m.rightColumn()
	sep := stDim.Render(strings.TrimRight(strings.Repeat("│\n", m.bodyH()), "\n"))
	body := lipgloss.JoinHorizontal(lipgloss.Top, m.listVP.View(), " ", sep, " ", right)
	return m.ti.View() + "\n" + body + "\n" + m.footer()
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
	return previewHeader(r.e.pr, w) + "\n\n" + m.prevVP.View()
}

func (m model) footer() string {
	status := ""
	switch {
	case m.netErr != "":
		status = truncate(m.netErr, m.width/2)
	case m.refreshing:
		status = "refreshing…"
	}
	f := m.help.View(m.keys)
	if status != "" {
		f += "  " + stDim.Render(status)
	}
	return f
}

// promptText builds the textinput prompt, with an orange "(dev)" marker on
// non-release builds.
func promptText() string {
	if strings.HasPrefix(version, "v") {
		return stPrompt.Render("gotopr ❯ ")
	}
	return stPrompt.Render("gotopr (") + stDev.Render("dev") + stPrompt.Render(") ❯ ")
}

// runAction executes the queued herdr CLI call after the TUI has exited.
func runAction(action []string) {
	if action == nil {
		return
	}
	_ = exec.Command(herdrBin(), action...).Run()
}
