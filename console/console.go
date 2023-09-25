package console

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gdamore/tcell/v2"
	"github.com/pkg/errors"
	"github.com/rivo/tview"

	"github.com/streamdal/snitch-cli/config"
	"github.com/streamdal/snitch-cli/types"
)

const (
	MenuString = `Q ["Q"][darkcyan]Quit[white][""]  ` +
		`S ["S"][darkcyan]Select Component[white][""]  ` +
		`R ["R"][darkcyan]Set Sample Rate[white][""]  ` +
		`F ["F"][darkcyan]Filter[white][""]  ` +
		`P ["P"][darkcyan]Pause[white][""]  ` +
		`/ ["Search"][darkcyan]Search[white][""]`

	PrimitiveInfoModal  = "info_modal"
	PrimitiveRetryModal = "retry_modal"
	PrimitiveErrorModal = "error_modal"
	PrimitiveList       = "list"
	PrimitivePeekView   = "peek_view"
	PrimitiveFilter     = "filter"
	PrimitiveSearch     = "search"

	PageConnectionAttempt = "page_" + PrimitiveInfoModal
	PageConnectionRetry   = "page_" + PrimitiveRetryModal
	PageSelectComponent   = "page_" + PrimitiveList
	PagePeekError         = "page_" + PrimitiveErrorModal
	PagePeekView          = "page_" + PrimitivePeekView
	PageFilter            = "page_" + PrimitiveFilter
	PageSearch            = "page_" + PrimitiveSearch
)

type Console struct {
	app     *tview.Application
	layout  *tview.Flex
	menu    *tview.TextView
	pages   *tview.Pages
	options *Options
	log     *log.Logger
	started bool
}

type Options struct {
	Config *config.Config
	Logger *log.Logger
}

func New(opts *Options) (*Console, error) {
	if err := validateOptions(opts); err != nil {
		return nil, errors.Wrap(err, "unable to validate config")
	}

	c := &Console{
		options: opts,
		log:     opts.Logger.WithPrefix("console"),
	}

	if err := c.initializeComponents(); err != nil {
		return nil, errors.Wrap(err, "unable to initialize components")
	}

	return c, nil
}

func (c *Console) SetInputCapture(f func(event *tcell.EventKey) *tcell.EventKey) {
	c.app.SetInputCapture(f)
}

func (c *Console) GetInputCapture() func(event *tcell.EventKey) *tcell.EventKey {
	return c.app.GetInputCapture()
}

func (c *Console) ToggleAllMenuHighlights() {
	c.app.QueueUpdateDraw(func() {
		c.menu.Highlight(c.menu.GetHighlights()...)
	})
}

func (c *Console) ToggleMenuHighlight(regions ...string) {
	c.app.QueueUpdateDraw(func() {
		c.menu.Highlight(regions...)
	})
}

func (c *Console) SetMenuEntryOn(item string) {
	c.toggleMenuEntry(item, true)
}

func (c *Console) SetMenuEntryOff(item string) {
	c.toggleMenuEntry(item, false)
}

func (c *Console) toggleMenuEntry(text string, on bool) {
	menu := c.menu.GetText(false)

	replaceOld := "[darkcyan]" + text
	replaceNew := "[lightcyan]" + text + "[-]"

	var updatedMenu string

	if !on {
		replaceOld = "[lightcyan]" + text + "[-]"
		replaceNew = "[darkcyan]" + text
	}

	updatedMenu = strings.Replace(menu, replaceOld, replaceNew, -1)

	c.app.QueueUpdateDraw(func() {
		c.menu.Clear()
		fmt.Fprint(c.menu, updatedMenu)
	})
}

func (c *Console) DisplayFilter(defaultValue string, answerCh chan<- string) {
	c.Start()

	// Remove all menu highlights - you cannot access menu while in filter view
	c.app.QueueUpdateDraw(func() {
		c.menu.Highlight()
	})

	var hit bool
	var input string

	form := tview.NewForm().
		AddInputField("", defaultValue, 30, nil, func(text string) {
			hit = true
			input = text
		}).
		AddButton("OK", func() {
			// Use the original value if te user didn't edit input field
			if !hit {
				input = defaultValue
			}

			answerCh <- input
		}).
		AddButton("Reset", func() {
			answerCh <- ""
		}).
		AddButton("Cancel", func() {
			// Return the original value
			answerCh <- defaultValue
		})

	form.SetBorder(true).SetTitle("Filter")
	form.SetButtonsAlign(tview.AlignCenter)

	inputDialog := Center(form, 36, 7)
	c.pages.AddPage(PageFilter, inputDialog, true, true)
}

func (c *Console) DisplaySearch(defaultValue string, answerCh chan<- string) {
	c.Start()

	// Remove all menu highlights - you cannot access menu while in search view
	c.app.QueueUpdateDraw(func() {
		c.menu.Highlight()
	})

	var hit bool
	var input string

	form := tview.NewForm().
		AddInputField("", defaultValue, 30, nil, func(text string) {
			hit = true
			input = text
		}).
		AddButton("OK", func() {
			// Use the original value if te user didn't edit input field
			if !hit {
				input = defaultValue
			}

			answerCh <- input
		}).
		AddButton("Reset", func() {
			answerCh <- ""
		}).
		AddButton("Cancel", func() {
			// Return the original value
			answerCh <- defaultValue
		})

	form.SetBorder(true).SetTitle("Search")
	form.SetButtonsAlign(tview.AlignCenter)

	inputDialog := Center(form, 36, 7)
	c.pages.AddPage(PageSearch, inputDialog, true, true)
}

// DisplayPeek will display peek + write any actions we receive from the user
// to the action channel; the action channel is read by the peek() method.
// Accepts an _optional_ pagePeek to facilitate re-use of the peek view. This
// is needed so that when filter/pause is applied, the peek view retains the
// data captured within it.
func (c *Console) DisplayPeek(pagePeek *tview.TextView, title string, actionCh chan<- *types.Action) *tview.TextView {
	c.Start()

	if pagePeek == nil {
		pagePeek = tview.NewTextView()
		pagePeek.SetBorder(true)
		pagePeek.SetDynamicColors(true)
		pagePeek.SetMaxLines(c.options.Config.MaxOutputLines)
	}

	// Always update title
	pagePeek.SetTitle(title)

	c.menu.Highlight("Q", "S", "P", "R", "F", "Search")

	c.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune && event.Rune() == 'q' {
			actionCh <- &types.Action{
				Step: types.StepQuit,
			}
		}

		if event.Key() == tcell.KeyRune && event.Rune() == 's' {
			actionCh <- &types.Action{
				Step: types.StepSelect,
			}
		}

		if event.Key() == tcell.KeyRune && event.Rune() == 'p' {
			actionCh <- &types.Action{
				Step: types.StepPause,
			}
		}

		// Pass along PeekComponent name so that once filter view is done,
		// peek knows what component it was operating on.
		if event.Key() == tcell.KeyRune && event.Rune() == 'f' {
			actionCh <- &types.Action{
				Step:          types.StepFilter,
				PeekComponent: title,
			}
		}

		// Pass along PeekComponent name so that once search view is done,
		// peek knows what component it was operating on.
		if event.Key() == tcell.KeyRune && event.Rune() == '/' {
			actionCh <- &types.Action{
				Step:          types.StepSearch,
				PeekComponent: title,
			}
		}

		return event
	})

	c.pages.AddPage(PagePeekView, pagePeek, true, true)
	c.pages.SwitchToPage(PagePeekView)

	return pagePeek
}

func (c *Console) Start() {
	if c.started {
		return
	}

	go func() {
		c.app.SetRoot(c.layout, true).SetFocus(c.pages)

		if err := c.app.Run(); err != nil {
			panic("unable to .Run app")
		}
	}()

	time.Sleep(100 * time.Millisecond) // Hack to give tview app enough time to start

	c.started = true

	return
}

// DisplayRetryModal will display a modal with a given message + retry/quit buttons.
func (c *Console) DisplayRetryModal(msg, pageName string, answerCh chan bool) {
	c.Start()

	retryModal := tview.NewModal().
		SetText(msg).
		AddButtons([]string{"Retry", "Quit"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonIndex == 0 {
				answerCh <- true
			} else {
				answerCh <- false
			}
		})

	c.pages.AddPage(pageName, retryModal, true, true)

	c.app.QueueUpdateDraw(func() {
		c.pages.SwitchToPage(pageName)
	})
}

// DisplayInfoModal will display an animated modal with the given message.
// InputCh is used by caller to indicate that the modal can be closed (in this
// case, it will cause the method to stop the animation goroutine).
// OutputCh is used by method to inform caller that the user has exited the modal.
func (c *Console) DisplayInfoModal(msg string, inputCh, outputCh chan error) {
	c.Start()

	// Needed to improve the way the "animation" looks
	msg = msg + " "

	infoModal := tview.NewModal().
		SetText(msg).
		AddButtons([]string{"Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonIndex == 0 {
				outputCh <- errors.New("user cancelled modal")
			}
		})

	// First time seeing this component - launch progress update goroutine; once
	// goroutine exits, it removes the component from the primitives map as well
	go func() {
		animationElements := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		ticker := time.NewTicker(time.Millisecond * 100)

		iter := 0

		defer ticker.Stop()

	MAIN:
		for {
			select {
			case <-inputCh:
				// Told to quit
				break MAIN
			case <-ticker.C:
				if iter == len(animationElements) {
					iter = 0
				}

				c.app.QueueUpdateDraw(func() {
					infoModal.SetText(fmt.Sprintf("%s[gray]%s[white]", msg, animationElements[iter]))
				})

				iter += 1
			}
		}
	}()

	c.pages.AddPage(PageConnectionAttempt, infoModal, true, true)
	c.pages.SwitchToPage(PageConnectionAttempt)
}

func (c *Console) Stop() {
	if c.started {
		c.app.Stop()
	}
}

func (c *Console) DisplayErrorModal(msg string) {
	c.Start()

	// There is no need to re-use the component here, as it does not get updates

	retryModal := tview.NewModal().
		SetText(msg).
		AddButtons([]string{"Quit"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonIndex == 0 {
				c.app.Stop()
			}
		})

	c.pages.AddPage(PagePeekError, retryModal, true, true)
	c.pages.SwitchToPage(PagePeekError)
}

func Center(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 1, true).
			AddItem(nil, 0, 1, false), width, 1, true).
		AddItem(nil, 0, 1, false)
}

func (c *Console) Redraw(f func()) {
	c.app.QueueUpdateDraw(f)
}

// DisplaySelectList will display a list of items and return the select item on the
// output channel
func (c *Console) DisplaySelectList(title string, itemMap map[string]string, output chan<- string) {
	selectComponent := tview.NewList()
	selectComponent.SetBorder(true)
	selectComponent.SetTitle(title)

	// I spent a good 15 minutes trying to find how to dynamically generate a
	// rune from an int - couldn't find anything. So, this is what we're doing.
	// ¯\_(ツ)_/¯
	i := 0
	shortcuts := []rune{'1', '2', '3', '4', '5', '6', '7', '8', '9'}

	for tmpName, tmpDesc := range itemMap {
		name := tmpName
		desc := tmpDesc

		var shortcut rune

		if i >= len(shortcuts) {
			shortcut = '0'
		} else {
			shortcut = shortcuts[i]
		}

		selectComponent.AddItem(name, desc, shortcut, func() {
			output <- name
		})

		i++
	}

	// Put this in a flex primitive so we can center it
	selectComponentFlex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(selectComponent, 10, 1, true).
			AddItem(nil, 0, 1, false), 48, 1, true).
		AddItem(nil, 0, 1, false)

	// Add Page
	c.pages.AddPage(PageSelectComponent, selectComponentFlex, true, true)
	c.pages.SwitchToPage(PageSelectComponent)
}

func (c *Console) initializeComponents() error {
	c.app = tview.NewApplication()
	c.pages = tview.NewPages()

	// Only highlight Quit at this time
	c.menu = c.newMenu()
	c.menu.Highlight("Q")

	// Create Layout
	c.layout = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(c.pages, 0, 1, true).
		AddItem(c.menu, 1, 1, false)

	return nil
}

func (c *Console) newMenu() *tview.TextView {
	menu := tview.NewTextView().SetWrap(false).SetDynamicColors(true)

	fmt.Fprint(menu, MenuString)

	return menu
}

func validateOptions(opts *Options) error {
	if opts == nil {
		return errors.New("options cannot be nil")
	}

	if opts.Config == nil {
		return errors.New(".Config cannot be nil")
	}

	if opts.Logger == nil {
		return errors.New(".Logger cannot be nil")
	}

	return nil
}
