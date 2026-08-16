// Package main provides a GTK4-based UI for golocate.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/RelicOfTesla/golocate/internal/client"
	"github.com/RelicOfTesla/golocate/pkg/errors"
	"github.com/RelicOfTesla/golocate/pkg/index"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"gopkg.in/yaml.v3"
)

// Socket path for the client connection
var socketPath string

// Pagination state
type PaginationState struct {
	currentPage int
	totalPages  int
	totalResults int
	pageSize    int
	currentQuery string
	isLoading   bool
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

// Results table (name / path / size / modtime)
var (
	resultsStore *gtk.ListStore
	resultsTree  *gtk.TreeView
)

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
	entry := gtk.NewEntry()
	entry.SetPlaceholderText("Search files...")
	entry.SetHExpand(true)
	searchBox.Append(entry)
	
	// Create search button
	searchBtn := gtk.NewButtonWithLabel("Search")
	searchBox.Append(searchBtn)
	
	// Create status button
	statusBtn := gtk.NewButtonWithLabel("Status")
	searchBox.Append(statusBtn)
	
	// Create config button
	configBtn := gtk.NewButtonWithLabel("Config")
	searchBox.Append(configBtn)
	
	mainBox.Append(searchBox)
	
	// Create results info label
	resultsInfoLabel = gtk.NewLabel("")
	resultsInfoLabel.SetHAlign(gtk.AlignStart)
	mainBox.Append(resultsInfoLabel)
	
	// Create scrolled window for results
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	
	// Create results table (name / path / size / modtime)
	resultsStore = gtk.NewListStore([]coreglib.Type{
		coreglib.TypeString, coreglib.TypeString, coreglib.TypeString, coreglib.TypeString,
	})
	resultsTree = gtk.NewTreeViewWithModel(resultsStore)
	resultsTree.SetHeadersVisible(true)
	
	nameCol := gtk.NewTreeViewColumn()
	nameCol.SetTitle("文件名")
	nameCol.SetExpand(true)
	nameRenderer := gtk.NewCellRendererText()
	nameCol.PackStart(nameRenderer, false)
	nameCol.AddAttribute(nameRenderer, "text", 0)
	resultsTree.AppendColumn(nameCol)
	
	pathCol := gtk.NewTreeViewColumn()
	pathCol.SetTitle("路径")
	pathCol.SetExpand(true)
	pathRenderer := gtk.NewCellRendererText()
	pathCol.PackStart(pathRenderer, false)
	pathCol.AddAttribute(pathRenderer, "text", 1)
	resultsTree.AppendColumn(pathCol)
	
	sizeCol := gtk.NewTreeViewColumn()
	sizeCol.SetTitle("大小")
	sizeCol.SetResizable(true)
	sizeRenderer := gtk.NewCellRendererText()
	sizeCol.PackStart(sizeRenderer, false)
	sizeCol.AddAttribute(sizeRenderer, "text", 2)
	resultsTree.AppendColumn(sizeCol)
	
	timeCol := gtk.NewTreeViewColumn()
	timeCol.SetTitle("修改时间")
	timeCol.SetResizable(true)
	timeRenderer := gtk.NewCellRendererText()
	timeCol.PackStart(timeRenderer, false)
	timeCol.AddAttribute(timeRenderer, "text", 3)
	resultsTree.AppendColumn(timeCol)
	
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
	
	// Set window child
	win.SetChild(mainBox)
	
	// Show window
	win.Present()
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
	
	// Search
	result, err := c.SearchFast(query, index.SearchOptions{
		Limit:  pagination.pageSize,
		Offset: offset,
	})
	
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
	if len(result.Entries) == 0 {
		resultsInfoLabel.SetText("未找到结果")
		updatePaginationUI()
		return
	}
	
	for _, r := range result.Entries {
		iter := resultsStore.Append()
		resultsStore.SetValue(iter, 0, coreglib.NewValue(r.Name))
		resultsStore.SetValue(iter, 1, coreglib.NewValue(r.Path))
		resultsStore.SetValue(iter, 2, coreglib.NewValue(formatSize(r.Size)))
		resultsStore.SetValue(iter, 3, coreglib.NewValue(formatModTime(r.ModTime)))
	}
	
	// Update pagination state
	pagination.totalResults = result.Total
	pagination.totalPages = (result.Total + pagination.pageSize - 1) / pagination.pageSize
	if pagination.totalPages < 1 {
		pagination.totalPages = 1
	}
	
	// Update UI
	resultsInfoLabel.SetText(fmt.Sprintf("共 %d 条（当前页 %d 条，第 %d/%d 页）",
		pagination.totalResults, result.Count, pagination.currentPage, pagination.totalPages))
	updatePaginationUI()
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
		nil,
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
		nil,
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
