// Package main provides a GTK4-based UI for golocate.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/RelicOfTesla/golocate/internal/client"
	"github.com/RelicOfTesla/golocate/pkg/errors"
	"github.com/RelicOfTesla/golocate/pkg/index"
	"github.com/RelicOfTesla/golocate/pkg/search"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"gopkg.in/yaml.v3"
)

// Socket path for the client connection
var socketPath string

// mainWindow is the top-level application window, used as dialog parent
// (gotk4's NewMessageDialog requires a non-nil parent Window).
var mainWindow *gtk.ApplicationWindow

// Pagination state
type PaginationState struct {
	currentPage  int
	totalPages   int
	totalResults int
	pageSize     int
	currentQuery string
	isLoading    bool
}

var pagination = PaginationState{
	currentPage:  1,
	totalPages:   1,
	totalResults: 0,
	pageSize:     100,
	currentQuery: "",
	isLoading:    false,
}

// UI elements for pagination
var (
	paginationBox    *gtk.Box
	firstPageBtn     *gtk.Button
	prevPageBtn      *gtk.Button
	nextPageBtn      *gtk.Button
	lastPageBtn      *gtk.Button
	pageLabel        *gtk.Label
	pageEntry        *gtk.Entry
	resultsInfoLabel *gtk.Label
	loadingSpinner   *gtk.Spinner
)

// Results table (name / path / size / modtime / match)
var (
	resultsStore *gtk.ListStore
	resultsTree  *gtk.TreeView
	nameCol      *gtk.TreeViewColumn
	pathCol      *gtk.TreeViewColumn
	sizeCol      *gtk.TreeViewColumn
	timeCol      *gtk.TreeViewColumn
	matchCol     *gtk.TreeViewColumn
)

// Current page results + sort state (for click-to-sort and export)
var (
	currentEntries []*index.Entry
	currentMatches []*client.ContentMatch // content search matches (content mode)
	sortField      string                 // "", "name", "path", "size", "time"
	sortOrder      string                 // "asc", "desc"
)

// Search history state
var (
	searchHistory []string
	historyStore  *gtk.ListStore
	searchEntry   *gtk.Entry
)

// Advanced search options
var (
	ignoreCaseBtn *gtk.CheckButton
	regexBtn      *gtk.CheckButton
	contentBtn    *gtk.CheckButton
	contentEntry  *gtk.Entry       // 独立内容关键词（与 searchEntry 路径过滤 AND，跟随 H5）
	typesEntry    *gtk.Entry       // 类型过滤（逗号/空格分隔，可选）
	dedupeBtn     *gtk.CheckButton // 硬链接去重
	exportBtn     *gtk.Button
)

// parseTypeList splits a comma/space separated type filter (e.g. "go, md").
func parseTypeList(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	}) {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// historyFile returns the path of the search-history file.
func historyFile() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return base + "/golocate/search_history.txt"
}

// loadHistory loads saved search history from disk.
func loadHistory() {
	searchHistory = nil
	data, err := os.ReadFile(historyFile())
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && len(searchHistory) < 20 {
			searchHistory = append(searchHistory, line)
		}
	}
}

// saveHistory persists search history to disk.
func saveHistory() {
	dir := filepath.Dir(historyFile())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	_ = os.WriteFile(historyFile(), []byte(histories()), 0644)
}

// histories joins the history into a text blob.
func histories() string {
	out := ""
	for _, h := range searchHistory {
		out += h + "\n"
	}
	return out
}

// refreshHistoryModel pushes history entries into the completion store.
func refreshHistoryModel() {
	historyStore.Clear()
	for _, h := range searchHistory {
		iter := historyStore.Append()
		historyStore.SetValue(iter, 0, coreglib.NewValue(h))
	}
}

func main() {
	app := gtk.NewApplication("com.github.golocate", 0)

	// Add --socket option
	app.AddMainOption("socket", 's', glib.OptionFlagNone, glib.OptionArgString, "Socket path or named pipe name", "PATH")

	// Handle command-line options
	app.ConnectHandleLocalOptions(func(options *glib.VariantDict) int {
		if v := options.LookupValue("socket", nil); v != nil {
			socketPath = v.String()
		}
		return -1 // Continue with default handling
	})

	app.Connect("activate", func() {
		createMainWindow(app)
	})

	// Run application
	status := app.Run(os.Args)
	if status > 0 {
		slog.Error("Application exited with status", "status", status)
		os.Exit(1)
	}
}

func createMainWindow(app *gtk.Application) {
	// Create main window
	win := gtk.NewApplicationWindow(app)
	mainWindow = win
	win.SetTitle("golocate - Fast File Search")
	win.SetDefaultSize(900, 700)

	// Create main container
	mainBox := gtk.NewBox(gtk.OrientationVertical, 10)
	mainBox.SetMarginTop(10)
	mainBox.SetMarginBottom(10)
	mainBox.SetMarginStart(10)
	mainBox.SetMarginEnd(10)

	// Create search box
	searchBox := gtk.NewBox(gtk.OrientationHorizontal, 10)

	// Create search entry
	searchEntry = gtk.NewEntry()
	searchEntry.SetPlaceholderText("Search files...")
	searchEntry.SetHExpand(true)
	searchBox.Append(searchEntry)
	entry := searchEntry

	// Search history completion
	historyStore = gtk.NewListStore([]coreglib.Type{coreglib.TypeString})
	loadHistory()
	refreshHistoryModel()
	completion := gtk.NewEntryCompletion()
	completion.SetModel(historyStore)
	completion.SetTextColumn(0)
	entry.SetCompletion(completion)

	// Create search button
	searchBtn := gtk.NewButtonWithLabel("Search")
	searchBox.Append(searchBtn)

	// Create status button
	statusBtn := gtk.NewButtonWithLabel("Status")
	searchBox.Append(statusBtn)

	// Create config button
	configBtn := gtk.NewButtonWithLabel("Config")
	searchBox.Append(configBtn)

	// Rebuild index button
	rebuildBtn := gtk.NewButtonWithLabel("重建索引")
	rebuildBtn.SetTooltipText("请求服务端重建索引")
	searchBox.Append(rebuildBtn)

	// Open buttons (operate on the selected result row)
	openBtn := gtk.NewButtonWithLabel("打开")
	openBtn.SetTooltipText("打开选中的文件 (双击结果行亦可)")
	searchBox.Append(openBtn)

	openDirBtn := gtk.NewButtonWithLabel("打开目录")
	openDirBtn.SetTooltipText("打开选中文件所在的目录")
	searchBox.Append(openDirBtn)

	// Advanced search options
	ignoreCaseBtn = gtk.NewCheckButtonWithLabel("忽略大小写")
	ignoreCaseBtn.SetActive(false)
	searchBox.Append(ignoreCaseBtn)

	regexBtn = gtk.NewCheckButtonWithLabel("正则")
	regexBtn.SetActive(false)
	searchBox.Append(regexBtn)

	contentBtn = gtk.NewCheckButtonWithLabel("内容搜索")
	contentBtn.SetActive(false)
	searchBox.Append(contentBtn)

	// 内容关键词输入（可选）：与 searchEntry 的路径过滤做 AND
	contentEntry = gtk.NewEntry()
	contentEntry.SetPlaceholderText("内容(可选)")
	contentEntry.SetWidthChars(14)
	searchBox.Append(contentEntry)

	// 文件类型过滤与硬链接去重（跟随 H5 高级过滤）
	typesEntry = gtk.NewEntry()
	typesEntry.SetPlaceholderText("类型(如 go,md)")
	typesEntry.SetWidthChars(12)
	searchBox.Append(typesEntry)

	dedupeBtn = gtk.NewCheckButtonWithLabel("去重(硬链接)")
	dedupeBtn.SetActive(false)
	searchBox.Append(dedupeBtn)

	// Export results button (saves current page as CSV)
	exportBtn = gtk.NewButtonWithLabel("导出 CSV")
	exportBtn.Connect("clicked", exportResults)
	searchBox.Append(exportBtn)

	mainBox.Append(searchBox)

	// Create results info label
	resultsInfoLabel = gtk.NewLabel("")
	resultsInfoLabel.SetHAlign(gtk.AlignStart)
	mainBox.Append(resultsInfoLabel)

	// Create scrolled window for results
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)

	// Create results table (name / path / size / modtime / match)
	resultsStore = gtk.NewListStore([]coreglib.Type{
		coreglib.TypeString, coreglib.TypeString, coreglib.TypeString, coreglib.TypeString, coreglib.TypeString,
	})
	resultsTree = gtk.NewTreeViewWithModel(resultsStore)
	resultsTree.SetHeadersVisible(true)

	nameCol = gtk.NewTreeViewColumn()
	nameCol.SetTitle("文件名")
	nameCol.SetResizable(true)
	nameCol.SetSizing(gtk.TreeViewColumnFixed)
	nameCol.SetFixedWidth(180)
	nameRenderer := gtk.NewCellRendererText()
	nameCol.PackStart(nameRenderer, false)
	nameCol.AddAttribute(nameRenderer, "text", 0)
	nameCol.ConnectClicked(func() { toggleSort("name") })
	resultsTree.AppendColumn(nameCol)

	pathCol = gtk.NewTreeViewColumn()
	pathCol.SetTitle("路径")
	pathCol.SetResizable(true)
	pathCol.SetSizing(gtk.TreeViewColumnFixed)
	pathCol.SetFixedWidth(400)
	pathRenderer := gtk.NewCellRendererText()
	pathCol.PackStart(pathRenderer, false)
	pathCol.AddAttribute(pathRenderer, "text", 1)
	pathCol.ConnectClicked(func() { toggleSort("path") })
	resultsTree.AppendColumn(pathCol)

	sizeCol = gtk.NewTreeViewColumn()
	sizeCol.SetTitle("大小")
	sizeCol.SetResizable(true)
	sizeRenderer := gtk.NewCellRendererText()
	sizeCol.PackStart(sizeRenderer, false)
	sizeCol.AddAttribute(sizeRenderer, "text", 2)
	sizeCol.SetSizing(gtk.TreeViewColumnFixed)
	sizeCol.SetFixedWidth(100)
	sizeCol.ConnectClicked(func() { toggleSort("size") })
	resultsTree.AppendColumn(sizeCol)

	timeCol = gtk.NewTreeViewColumn()
	timeCol.SetTitle("修改时间")
	timeCol.SetResizable(true)
	timeRenderer := gtk.NewCellRendererText()
	timeCol.PackStart(timeRenderer, false)
	timeCol.AddAttribute(timeRenderer, "text", 3)
	timeCol.SetSizing(gtk.TreeViewColumnFixed)
	timeCol.SetFixedWidth(150)
	timeCol.ConnectClicked(func() { toggleSort("time") })
	resultsTree.AppendColumn(timeCol)

	matchCol = gtk.NewTreeViewColumn()
	matchCol.SetTitle("匹配内容")
	matchCol.SetResizable(true)
	matchRenderer := gtk.NewCellRendererText()
	matchCol.PackStart(matchRenderer, false)
	matchCol.AddAttribute(matchRenderer, "text", 4)
	matchCol.SetSizing(gtk.TreeViewColumnFixed)
	matchCol.SetFixedWidth(300)
	resultsTree.AppendColumn(matchCol)

	scrolled.SetChild(resultsTree)

	mainBox.Append(scrolled)

	// Create pagination controls
	createPaginationControls(mainBox, entry)

	// Create loading spinner
	loadingSpinner = gtk.NewSpinner()
	loadingSpinner.SetHAlign(gtk.AlignCenter)
	loadingSpinner.SetVAlign(gtk.AlignCenter)

	// Create client with socket path
	c := client.New()
	if socketPath != "" {
		c.SetSocketPath(socketPath)
	}

	// Setup keyboard shortcuts
	setupKeyboardShortcuts(win, entry)

	// Search function
	doSearch := func() {
		query := entry.Text()
		if query == "" {
			resultsInfoLabel.SetText("请输入搜索关键词")
			resultsStore.Clear()
			return
		}

		// Reset to first page for new search
		if query != pagination.currentQuery {
			pagination.currentPage = 1
			pagination.currentQuery = query
		}

		// Save to search history (most recent first, deduped)
		var rest []string
		for _, h := range searchHistory {
			if h != query {
				rest = append(rest, h)
			}
		}
		searchHistory = append([]string{query}, rest...)
		if len(searchHistory) > 20 {
			searchHistory = searchHistory[:20]
		}
		refreshHistoryModel()
		saveHistory()

		performSearch(c, query)
	}

	// Connect search button
	searchBtn.Connect("clicked", doSearch)

	// Connect entry activate (Enter key)
	entry.Connect("activate", doSearch)

	// Connect status button
	statusBtn.Connect("clicked", func() {
		status, err := c.Status()
		if err != nil {
			// Check if it's a server not running error
			if errors.IsServerNotRunningError(err) {
				showErrorDialog(errors.GetFriendlyErrorMessage(err))
				return
			}
			showErrorDialog(fmt.Sprintf("Error: %v", err))
			return
		}

		// Display status
		running, _ := status["running"].(bool)
		var indexSize int
		if v, ok := status["index_size"].(float64); ok {
			indexSize = int(v)
		} else if v, ok := status["index_size"].(int); ok {
			indexSize = v
		}
		uptime, _ := status["uptime"].(string)
		pid, _ := status["pid"].(float64)

		var statusText string
		if running {
			statusText = fmt.Sprintf("Server 状态: 运行中\n\nPID: %.0f\n索引文件数: %d\n运行时间: %s", pid, indexSize, uptime)
		} else {
			statusText = "Server 状态: 未运行\n\n启动: golocated --service"
		}
		showInfoDialog(statusText)
	})

	// Connect config button
	configBtn.Connect("clicked", func() {
		showConfigDialog(win, c)
	})

	// Connect rebuild button
	rebuildBtn.Connect("clicked", func() {
		go func() {
			err := c.Build()
			glib.IdleAdd(func() bool {
				if err != nil {
					showErrorDialog(fmt.Sprintf("重建请求失败: %v", err))
				} else {
					showInfoDialog("索引重建已开始")
				}
				return false
			})
		}()
	})

	// Connect open buttons (selected row based)
	openBtn.Connect("clicked", func() {
		if p, ok := selectedResultPath(); ok {
			openWithSystemApp(p)
		} else {
			resultsInfoLabel.SetText("请先选择一个结果")
		}
	})
	openDirBtn.Connect("clicked", func() {
		if p, ok := selectedResultPath(); ok {
			openWithSystemApp(filepath.Dir(p))
		} else {
			resultsInfoLabel.SetText("请先选择一个结果")
		}
	})

	// Double-click a result row opens the file
	resultsTree.Connect("row-activated", func() {
		if p, ok := selectedResultPath(); ok {
			openWithSystemApp(p)
		}
	})

	// Set window child
	win.SetChild(mainBox)

	// Show window
	win.Present()
}

// selectedResultPath returns the path of the currently selected result row.
func selectedResultPath() (string, bool) {
	sel := resultsTree.Selection()
	if sel == nil {
		return "", false
	}
	// gotk4 v0.3.1: Selected() returns (iter, model, ok) — keep iter only.
	iter, _, _ := sel.Selected()
	if iter == nil {
		return "", false
	}
	v := resultsStore.Value(iter, 1) // column 1 = path
	// glib.Value.String() returns a single value (no ok).
	p := v.String()
	if p == "" {
		return "", false
	}
	return p, true
}

// openWithSystemApp opens the given file/directory with the platform's
// default application (xdg-open on Linux, open on macOS, explorer on Windows).
func openWithSystemApp(p string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", p)
	case "darwin":
		cmd = exec.Command("open", p)
	default:
		cmd = exec.Command("xdg-open", p)
	}
	if err := cmd.Start(); err != nil {
		showErrorDialog(fmt.Sprintf("打开失败: %v", err))
	}
}

func createPaginationControls(mainBox *gtk.Box, entry *gtk.Entry) {
	// Create pagination box
	paginationBox = gtk.NewBox(gtk.OrientationHorizontal, 5)
	paginationBox.SetHAlign(gtk.AlignCenter)
	paginationBox.SetMarginTop(10)
	paginationBox.SetMarginBottom(10)

	// First page button
	firstPageBtn = gtk.NewButtonWithLabel("◀◀ 首页")
	firstPageBtn.SetTooltipText("首页 (Home)")
	firstPageBtn.Connect("clicked", func() {
		goToFirstPage(entry)
	})
	paginationBox.Append(firstPageBtn)

	// Previous page button
	prevPageBtn = gtk.NewButtonWithLabel("◀ 上一页")
	prevPageBtn.SetTooltipText("上一页 (PageUp / ←)")
	prevPageBtn.Connect("clicked", func() {
		goToPrevPage(entry)
	})
	paginationBox.Append(prevPageBtn)

	// Page label
	pageLabel = gtk.NewLabel("第 1/1 页")
	pageLabel.SetMarginStart(10)
	pageLabel.SetMarginEnd(10)
	paginationBox.Append(pageLabel)

	// Page entry for jumping
	pageEntryBox := gtk.NewBox(gtk.OrientationHorizontal, 5)

	pageEntry = gtk.NewEntry()
	pageEntry.SetPlaceholderText("页码")
	pageEntry.SetWidthChars(6)
	pageEntry.SetInputPurpose(gtk.InputPurposeNumber)
	pageEntry.Connect("activate", func() {
		jumpToPage(entry)
	})
	pageEntryBox.Append(pageEntry)

	jumpBtn := gtk.NewButtonWithLabel("跳转")
	jumpBtn.Connect("clicked", func() {
		jumpToPage(entry)
	})
	pageEntryBox.Append(jumpBtn)

	paginationBox.Append(pageEntryBox)

	// Next page button
	nextPageBtn = gtk.NewButtonWithLabel("下一页 ▶")
	nextPageBtn.SetTooltipText("下一页 (PageDown / →)")
	nextPageBtn.Connect("clicked", func() {
		goToNextPage(entry)
	})
	paginationBox.Append(nextPageBtn)

	// Last page button
	lastPageBtn = gtk.NewButtonWithLabel("末页 ▶▶")
	lastPageBtn.SetTooltipText("末页 (End)")
	lastPageBtn.Connect("clicked", func() {
		goToLastPage(entry)
	})
	paginationBox.Append(lastPageBtn)

	// Initially hide pagination
	paginationBox.SetVisible(false)

	mainBox.Append(paginationBox)
}

func setupKeyboardShortcuts(win *gtk.ApplicationWindow, entry *gtk.Entry) {
	// Create event controller for key events
	controller := gtk.NewEventControllerKey()
	controller.Connect("key-pressed", func(keyval, keycode uint, state gdk.ModifierType) bool {
		// Don't handle shortcuts when typing in entry
		if entry.HasFocus() {
			// Allow Tab and Escape to navigate away
			if keyval == gdk.KEY_Tab || keyval == gdk.KEY_Escape {
				return false
			}
			// Allow Enter in page entry
			if keyval == gdk.KEY_Return && pageEntry.HasFocus() {
				jumpToPage(entry)
				return true
			}
			return false
		}

		// Don't handle shortcuts when loading
		if pagination.isLoading {
			return false
		}

		switch keyval {
		case gdk.KEY_Page_Down:
			goToNextPage(entry)
			return true
		case gdk.KEY_Page_Up:
			goToPrevPage(entry)
			return true
		case gdk.KEY_Left:
			goToPrevPage(entry)
			return true
		case gdk.KEY_Right:
			goToNextPage(entry)
			return true
		case gdk.KEY_Home:
			goToFirstPage(entry)
			return true
		case gdk.KEY_End:
			goToLastPage(entry)
			return true
		case gdk.KEY_F5:
			focusPageEntry()
			return true
		case gdk.KEY_g:
			if state&gdk.ControlMask != 0 || state&gdk.ModifierType(gdk.ControlMask) != 0 {
				focusPageEntry()
				return true
			}
		}

		return false
	})

	win.AddController(controller)
}

func focusPageEntry() {
	if paginationBox.IsVisible() {
		pageEntry.GrabFocus()
	}
}

func performSearch(c *client.Client, query string) {
	pagination.isLoading = true

	// Calculate offset
	offset := int64((pagination.currentPage - 1) * pagination.pageSize)

	// 内容搜索词：优先取独立内容输入框（与路径词 AND 跟随 H5）；否则兼容旧 contentBtn。
	contentKeyword := ""
	if contentEntry != nil {
		contentKeyword = strings.TrimSpace(contentEntry.Text())
	}
	contentMode := (contentBtn != nil && contentBtn.Active()) || contentKeyword != ""

	types := parseTypeList("")
	if typesEntry != nil {
		types = parseTypeList(typesEntry.Text())
	}
	dedupe := dedupeBtn != nil && dedupeBtn.Active()

	var entries []*index.Entry
	var matches []*client.ContentMatch
	total := 0
	var err error

	if contentMode {
		// Content search：keyword = 内容词；有独立内容输入框时 pattern = 路径过滤（AND），
		// 只有旧 contentBtn 勾选（无内容词）时保持纯内容搜索（pattern 为空）。
		kw := contentKeyword
		pattern := ""
		if kw == "" {
			kw = query // 旧 contentBtn 模式：直接用查询词
		} else {
			pattern = query
		}
		var cres *client.ContentSearchResult
		cres, err = c.SearchContent(pattern, kw, index.SearchOptions{
			Limit:       pagination.pageSize,
			Offset:      offset,
			IgnoreCase:  ignoreCaseBtn.Active(),
			PatternMode: modeFromUI(),
			Types:       types,
			Dedupe:      dedupe,
		})
		if err == nil {
			matches = cres.Matches
			total = cres.Total
			entries = make([]*index.Entry, 0, len(matches))
			for _, m := range matches {
				entries = append(entries, &index.Entry{
					Path: m.Path,
					Name: filepath.Base(m.Path),
				})
			}
		}
	} else {
		var result *client.SearchResult
		result, err = c.SearchFast(query, index.SearchOptions{
			Limit:       pagination.pageSize,
			Offset:      offset,
			IgnoreCase:  ignoreCaseBtn.Active(),
			PatternMode: modeFromUI(),
			Types:       types,
			Dedupe:      dedupe,
		})
		if err == nil {
			entries = result.Entries
			total = result.Total
		}
	}

	pagination.isLoading = false

	resultsStore.Clear()

	if err != nil {
		// Check if it's a server not running error
		if errors.IsServerNotRunningError(err) {
			resultsInfoLabel.SetText(errors.GetFriendlyErrorMessage(err))
			updatePaginationUI()
			return
		}
		resultsInfoLabel.SetText(fmt.Sprintf("Error: %v", err))
		updatePaginationUI()
		return
	}

	// Display results in the table
	if len(entries) == 0 {
		resultsInfoLabel.SetText("未找到结果")
		updatePaginationUI()
		return
	}

	currentEntries = entries
	currentMatches = matches
	if sortField != "" {
		applySort()
	}
	populateTable(resultsStore, currentEntries, matchTexts(currentMatches))

	// Update pagination state
	pagination.totalResults = total
	pagination.totalPages = (total + pagination.pageSize - 1) / pagination.pageSize
	if pagination.totalPages < 1 {
		pagination.totalPages = 1
	}

	// Update UI
	resultsInfoLabel.SetText(fmt.Sprintf("共 %d 条（当前页 %d 条，第 %d/%d 页）",
		pagination.totalResults, len(entries), pagination.currentPage, pagination.totalPages))
	updatePaginationUI()
}

// matchTexts formats content matches as "行N: 匹配行内容" for table display.
func matchTexts(matches []*client.ContentMatch) []string {
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, fmt.Sprintf("行%d: %s", m.LineNum, m.Line))
	}
	return out
}

// formatSize formats file size for display.
func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	if size < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(size)/(1024*1024*1024))
}

// formatModTime formats modification time as "YYYY-MM-DD HH:MM".
func formatModTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02 15:04")
}

// modeFromUI maps the regex checkbox to a search pattern mode.
// Empty means auto-detect on the server (wildcard vs substring).
func modeFromUI() index.PatternMode {
	if regexBtn != nil && regexBtn.Active() {
		return index.PatternModeExtendedRegex
	}
	return index.PatternMode("")
}

// populateTable fills the results tree store from entries.
// matches are parallel content-search texts (nil in normal mode).
func populateTable(store *gtk.ListStore, entries []*index.Entry, matches []string) {
	store.Clear()
	for i, r := range entries {
		iter := store.Append()
		matchText := "-"
		if matches != nil && i < len(matches) {
			matchText = matches[i]
		}
		store.SetValue(iter, 0, coreglib.NewValue(r.Name))
		store.SetValue(iter, 1, coreglib.NewValue(r.Path))
		store.SetValue(iter, 2, coreglib.NewValue(formatSize(r.Size)))
		store.SetValue(iter, 3, coreglib.NewValue(formatModTime(r.ModTime)))
		store.SetValue(iter, 4, coreglib.NewValue(matchText))
	}
}

// toggleSort toggles field/order on header click and re-renders.
func toggleSort(field string) {
	if sortField == field {
		if sortOrder == "asc" {
			sortOrder = "desc"
		} else {
			sortOrder = "asc"
		}
	} else {
		sortField = field
		sortOrder = "asc"
	}
	applySort()
}

// applySort sorts the current page entries in place and repopulates the table.
func applySort() {
	if sortField == "" {
		return
	}
	sortOpts := search.ParseSort(sortField + ":" + sortOrder)
	search.Sort(currentEntries, sortOpts)
	populateTable(resultsStore, currentEntries, matchTexts(currentMatches))
	updateSortHeader()
}

// exportResults writes the current page rows to a CSV file in Downloads.
func exportResults() {
	if len(currentEntries) == 0 {
		resultsInfoLabel.SetText("没有可导出的结果")
		return
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "Downloads")
	_ = os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, fmt.Sprintf("golocate_export_%s.csv", time.Now().Format("20060102_150405")))

	var sb strings.Builder
	if len(currentMatches) > 0 {
		// Content search export: include line numbers, matching text and context
		sb.WriteString("文件名,路径,行号,匹配内容,上下文\n")
		for i, r := range currentEntries {
			if i >= len(currentMatches) {
				break
			}
			m := currentMatches[i]
			ctx := make([]string, 0, len(m.Before)+len(m.After))
			ctx = append(ctx, m.Before...)
			ctx = append(ctx, m.After...)
			sb.WriteString(fmt.Sprintf("%s,%s,%d,\"%s\",\"%s\"\n",
				r.Name, r.Path, m.LineNum,
				strings.ReplaceAll(m.Line, "\"", "\"\""),
				strings.ReplaceAll(strings.Join(ctx, " | "), "\"", "\"\"")))
		}
	} else {
		sb.WriteString("文件名,路径,大小,修改时间\n")
		for _, r := range currentEntries {
			sb.WriteString(fmt.Sprintf("%s,%s,%s,%s\n",
				r.Name, r.Path, formatSize(r.Size), formatModTime(r.ModTime)))
		}
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		resultsInfoLabel.SetText(fmt.Sprintf("导出失败: %v", err))
		return
	}
	resultsInfoLabel.SetText(fmt.Sprintf("已导出 %d 条到 %s", len(currentEntries), path))
}

// updateSortHeader shows the current sort direction in the column titles.
func updateSortHeader() {
	arrow := ""
	if sortField != "" {
		if sortOrder == "asc" {
			arrow = " ↑"
		} else {
			arrow = " ↓"
		}
	}
	names := map[string]*gtk.TreeViewColumn{
		"name": nameCol, "path": pathCol, "size": sizeCol, "time": timeCol,
	}
	titles := map[string]string{
		"name": "文件名", "path": "路径", "size": "大小", "time": "修改时间",
	}
	for f, col := range names {
		if f == sortField {
			col.SetTitle(titles[f] + arrow)
		} else {
			col.SetTitle(titles[f])
		}
	}
}

func updatePaginationUI() {
	// Update page label
	pageLabel.SetText(fmt.Sprintf("第 %d/%d 页", pagination.currentPage, pagination.totalPages))

	// Update button sensitivity
	firstPageBtn.SetSensitive(pagination.currentPage > 1)
	prevPageBtn.SetSensitive(pagination.currentPage > 1)
	nextPageBtn.SetSensitive(pagination.currentPage < pagination.totalPages)
	lastPageBtn.SetSensitive(pagination.currentPage < pagination.totalPages)

	// Show pagination if there are results
	paginationBox.SetVisible(pagination.totalResults > 0)
}

func goToFirstPage(entry *gtk.Entry) {
	if pagination.currentPage == 1 || pagination.isLoading {
		return
	}
	pagination.currentPage = 1

	// Trigger search
	entry.Emit("activate")
}

func goToPrevPage(entry *gtk.Entry) {
	if pagination.currentPage == 1 || pagination.isLoading {
		return
	}
	pagination.currentPage--

	// Trigger search
	entry.Emit("activate")
}

func goToNextPage(entry *gtk.Entry) {
	if pagination.currentPage == pagination.totalPages || pagination.isLoading {
		return
	}
	pagination.currentPage++

	// Trigger search
	entry.Emit("activate")
}

func goToLastPage(entry *gtk.Entry) {
	if pagination.currentPage == pagination.totalPages || pagination.isLoading {
		return
	}
	pagination.currentPage = pagination.totalPages

	// Trigger search
	entry.Emit("activate")
}

func jumpToPage(entry *gtk.Entry) {
	text := pageEntry.Text()
	if text == "" {
		return
	}

	// Parse page number
	page := 0
	if _, err := fmt.Sscanf(text, "%d", &page); err != nil {
		pageEntry.SetText("")
		return
	}

	// Validate page number
	if page < 1 || page > pagination.totalPages {
		pageEntry.SetText(fmt.Sprintf("%d", pagination.currentPage))
		return
	}

	if page == pagination.currentPage || pagination.isLoading {
		return
	}

	pagination.currentPage = page
	pageEntry.SetText("")

	// Trigger search
	entry.Emit("activate")
}

// showConfigDialog shows the configuration dialog.
func showConfigDialog(parent *gtk.ApplicationWindow, c *client.Client) {
	// Create dialog
	dialog := gtk.NewDialog()
	dialog.SetTitle("服务器配置")
	dialog.SetModal(true)
	dialog.SetDefaultSize(600, 500)

	// Get content area
	contentArea := dialog.ContentArea()
	contentArea.SetMarginTop(10)
	contentArea.SetMarginBottom(10)
	contentArea.SetMarginStart(10)
	contentArea.SetMarginEnd(10)
	contentArea.SetSpacing(10)

	// Create label
	label := gtk.NewLabel("编辑配置文件 (YAML 格式):")
	label.SetHAlign(gtk.AlignStart)
	contentArea.Append(label)

	// Create scrolled window
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetHExpand(true)

	// Create text view
	textView := gtk.NewTextView()
	textView.SetEditable(true)
	textView.SetMonospace(true)
	textView.SetWrapMode(gtk.WrapWordChar)
	buffer := textView.Buffer()
	buffer.SetText("Loading configuration...")

	scrolled.SetChild(textView)
	contentArea.Append(scrolled)

	// Add buttons
	dialog.AddButton("取消", int(gtk.ResponseCancel))
	dialog.AddButton("保存", int(gtk.ResponseAccept))

	// Load config asynchronously
	go func() {
		config, err := c.GetConfig()
		if err != nil {
			glib.IdleAdd(func() bool {
				buffer.SetText(fmt.Sprintf("Error loading configuration: %v", err))
				return false
			})
			return
		}

		// Convert to YAML
		yamlData, err := yaml.Marshal(config)
		if err != nil {
			glib.IdleAdd(func() bool {
				buffer.SetText(fmt.Sprintf("Error converting to YAML: %v", err))
				return false
			})
			return
		}

		// Update UI in main thread
		glib.IdleAdd(func() bool {
			buffer.SetText(string(yamlData))
			return false
		})
	}()

	// Show dialog
	dialog.Present()

	// Handle response
	dialog.Connect("response", func(dialog *gtk.Dialog, responseID int) {
		if responseID == int(gtk.ResponseAccept) {
			// Save config
			startIter, endIter := buffer.Bounds()
			yamlText := buffer.Text(startIter, endIter, false)

			go func() {
				err := c.SetConfig(yamlText)
				glib.IdleAdd(func() bool {
					if err != nil {
						showErrorDialog(fmt.Sprintf("Error saving configuration: %v", err))
					} else {
						showInfoDialog("Configuration saved successfully!")
					}
					return false
				})
			}()
		}

		// Close dialog
		dialog.Close()
	})
}

// showErrorDialog shows an error dialog.
func showErrorDialog(message string) {
	dialog := gtk.NewMessageDialog(
		&mainWindow.Window,
		gtk.DialogDestroyWithParent,
		gtk.MessageError,
		gtk.ButtonsOK,
	)
	dialog.SetTitle("Error")
	dialog.SetMarkup(message)
	dialog.Connect("response", func(dialog *gtk.MessageDialog) {
		dialog.Close()
	})
	dialog.Present()
}

// showInfoDialog shows an info dialog.
func showInfoDialog(message string) {
	dialog := gtk.NewMessageDialog(
		&mainWindow.Window,
		gtk.DialogDestroyWithParent,
		gtk.MessageInfo,
		gtk.ButtonsOK,
	)
	dialog.SetTitle("Info")
	dialog.SetMarkup(message)
	dialog.Connect("response", func(dialog *gtk.MessageDialog) {
		dialog.Close()
	})
	dialog.Present()
}
