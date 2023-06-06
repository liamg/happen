package gui

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	_ "github.com/gdamore/tcell/v2/encoding"
	"github.com/liamg/happen/feed"
	"github.com/mattn/go-runewidth"
	"github.com/skratchdot/open-golang/open"
)

type GUI struct {
	filtering     bool
	filter        string
	filterEditing bool
	interacting   bool
	scroll        struct {
		visible   int
		offset    int // <- number of items which are hidden from the top
		selection int // <- index of the selected item
	}
	screen     tcell.Screen
	dataMu     sync.Mutex
	items      []feed.Item
	filtered   []feed.Item
	config     *feed.Config
	selectedID string
	lastUpdate time.Time
	updateMu   sync.Mutex
}

const (
	helpText = "q - exit | j/k/up/down - select | enter - open | esc - clear | / - filter | r - refresh"
)

func Create(conf *feed.Config) (*GUI, error) {

	screen, err := tcell.NewScreen()
	if err != nil {
		return nil, fmt.Errorf("failed to create screen: %w", err)
	}

	if e := screen.Init(); e != nil {
		return nil, fmt.Errorf("failed to initialise screen: %w", e)
	}

	return &GUI{
		screen: screen,
		config: conf,
	}, nil
}

func (g *GUI) Close() {
	g.screen.Fini()
}

func (g *GUI) Run(ctx context.Context) {

	g.screen.EnableMouse()

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	go g.Update()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ticker.C:
				g.updateMu.Lock()
				if time.Since(g.lastUpdate) < g.config.PollInterval {
					g.screen.PostEvent(tickEvent{
						t: time.Now(),
					})
					g.updateMu.Unlock()
					continue
				}
				g.updateMu.Unlock()
				go g.Update()
				g.screen.PostEvent(tickEvent{
					t: time.Now(),
				})
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		switch ev := g.screen.PollEvent().(type) {
		case tickEvent:
			g.Redraw()
		case updateEvent:
			g.applyFilter(g.filter, g.filtering)
			g.interacting = false
			g.Redraw()
		case *tcell.EventResize:
			g.screen.Sync()
			g.Redraw()
		case *tcell.EventMouse:
			if ev.Buttons()&tcell.WheelUp != 0 {
				g.changeSelection(-1, !g.interacting)
			} else if ev.Buttons()&tcell.WheelDown != 0 {
				g.changeSelection(1, !g.interacting)
			} else {
				continue
			}
			g.delayUpdate()
			g.Redraw()
		case *tcell.EventKey:
			switch ev.Key() {
			case tcell.KeyEscape:
				g.applyFilter("", false)
				g.interacting = false
				g.Redraw()
				continue
			case tcell.KeyRune:
				switch ev.Rune() {
				case 'o':
					if g.interacting {
						if g.scroll.selection < len(g.filtered) {
							g.openLink(g.filtered[g.scroll.selection].Url)
						}
					}
				case '/':
					g.startFilterInput()
					g.interacting = false
				case 'r':
					go g.Update()
					g.changeSelection(len(g.filtered), false)
					g.Redraw()
				case 'q':
					return
				case 'j':
					g.changeSelection(1, false)
				case 'k':
					g.changeSelection(-1, false)
				case 'g', '0':
					g.changeSelection(-len(g.filtered), false)
				case 'G', '$':
					g.changeSelection(len(g.filtered), false)
				}
			case tcell.KeyPgDn:
				g.changeSelection(g.scroll.visible, false)
			case tcell.KeyPgUp:
				g.changeSelection(-g.scroll.visible, false)
			case tcell.KeyEnd:
				g.changeSelection(len(g.filtered), false)
			case tcell.KeyHome:
				g.changeSelection(-len(g.filtered), false)
			case tcell.KeyDown:
				g.changeSelection(1, false)
			case tcell.KeyUp:
				g.changeSelection(-1, false)
			case tcell.KeyEnter:
				if g.interacting {
					if g.scroll.selection < len(g.filtered) {
						g.openLink(g.filtered[g.scroll.selection].Url)
					}
				}
			}
			g.delayUpdate()
			g.interacting = true
			g.Redraw()
		}
	}

}

func (g *GUI) delayUpdate() {
	g.updateMu.Lock()
	g.lastUpdate = time.Now()
	g.updateMu.Unlock()
}

type tickEvent struct {
	t time.Time
}

func (e tickEvent) When() time.Time {
	return e.t
}

type updateEvent struct {
	t time.Time
}

func (e updateEvent) When() time.Time {
	return e.t
}

func (g *GUI) startFilterInput() {
	g.dataMu.Lock()
	g.filterEditing = true
	g.dataMu.Unlock()
	defer func() {
		g.dataMu.Lock()
		g.filterEditing = false
		g.dataMu.Unlock()
	}()

	var filter string
	g.applyFilter(filter, true)
	g.Redraw()

	for {
		switch ev := g.screen.PollEvent().(type) {
		case tickEvent:
		case updateEvent:
			g.applyFilter(filter, true)
		case *tcell.EventResize:
			g.screen.Sync()
		case *tcell.EventKey:
			switch ev.Key() {
			case tcell.KeyEscape:
				g.applyFilter("", false)
				return
			case tcell.KeyEnter:
				if filter == "" {
					g.applyFilter("", false)
				}
				return
			case tcell.KeyBackspace, tcell.KeyBackspace2:
				if len(filter) > 0 {
					filter = filter[:len(filter)-1]
				}
			case tcell.KeyRune:
				filter += string(ev.Rune())
			}
			g.applyFilter(filter, true)
		}
		g.Redraw()
	}
}

func (g *GUI) applyFilter(filter string, filtering bool) {
	g.dataMu.Lock()
	defer g.dataMu.Unlock()
	g.filtering = filtering
	if !filtering {
		filter = ""
	}
	var filtered = make([]feed.Item, 0, len(g.items))
	filter = strings.ToLower(filter)
	for _, item := range g.items {
		switch {
		case filter == "":
			filtered = append(filtered, item)
		case strings.Contains(strings.ToLower(item.Title), filter):
			filtered = append(filtered, item)
		case strings.Contains(strings.ToLower(item.Description), filter):
			filtered = append(filtered, item)
		case strings.Contains(strings.ToLower(item.Source.Name), filter):
			filtered = append(filtered, item)
		case strings.Contains(strings.ToLower(item.Url), filter):
			filtered = append(filtered, item)
		}
	}
	before := len(g.filtered)
	g.filtered = filtered
	if before != len(filtered) {
		g.selectByID(g.selectedID)
		g.changeSelection(0, false)
	}
	g.filter = filter
}

func (g *GUI) openLink(url string) {
	_ = open.Start(url)
}

func (g *GUI) selectByID(id string) {
	if id == "" {
		return
	}
	to := -1
	for i, item := range g.filtered {
		if item.ID == id {
			to = i
			g.selectedID = id
			break
		}
	}
	if to == -1 {
		if len(g.filtered) > 0 {
			g.selectedID = g.filtered[0].ID
			to = 0
		} else {
			return
		}
	}
	if to < g.scroll.offset {
		g.scroll.offset = to
	} else if to >= g.scroll.offset+g.scroll.visible {
		g.scroll.offset = to - g.scroll.visible + 1
	}
	g.scroll.selection = to
}

func (g *GUI) changeSelection(change int, force bool) {
	//invert the change as it's easier to think about this way
	change = -change
	to := g.scroll.selection
	if force {
		if change > 0 && to < g.scroll.visible {
			to = g.scroll.visible
		}
		if change < 0 && to >= len(g.filtered)-g.scroll.visible {
			to = len(g.filtered) - g.scroll.visible - 1
		}
	}
	to += change
	if to < 0 {
		to = 0
	} else if to >= len(g.filtered) {
		to = len(g.filtered) - 1
	}
	if to < g.scroll.offset {
		g.scroll.offset = to
	} else if to >= g.scroll.offset+g.scroll.visible {
		g.scroll.offset = to - g.scroll.visible + 1
	}
	if len(g.filtered) > 0 && to < len(g.filtered) {
		g.selectedID = g.filtered[to].ID
	} else {
		g.selectedID = ""
	}
	g.scroll.selection = to
}

func (g *GUI) Update() {

	g.updateMu.Lock()
	defer g.updateMu.Unlock()
	defer func() { g.lastUpdate = time.Now() }()
	g.lastUpdate = time.Time{}

	mgr := feed.New(g.config)
	items, err := mgr.Read()
	if err != nil {
		// TODO: handle error and print details
		return
	}

	g.dataMu.Lock()
	g.items = items
	g.dataMu.Unlock()

	_ = g.screen.PostEvent(updateEvent{
		t: time.Now(),
	})
}

func (g *GUI) Redraw() {

	g.dataMu.Lock()
	defer g.dataMu.Unlock()

	g.screen.Clear()

	_, h := g.screen.Size()

	itemHeight := 2
	if g.config.ShowDescriptions {
		itemHeight++
	}

	g.scroll.visible = h / itemHeight

	var drawn int

	var maxBadge int
	for _, src := range g.config.Sources {
		if len(src.Name) > maxBadge {
			maxBadge = len(src.Name)
		}
	}
	if g.config.MaxBadgeSize > 0 && maxBadge > g.config.MaxBadgeSize {
		maxBadge = g.config.MaxBadgeSize
	}

	skip := g.scroll.offset
	for i, item := range g.filtered {

		if drawn >= g.scroll.visible {
			break
		}

		if skip > 0 {
			skip--
			continue
		}

		selected := g.interacting && i == g.scroll.selection

		y := drawn * itemHeight
		if y+itemHeight > h {
			break
		}
		y = h - y - itemHeight
		drawn++

		titleStyle := tcell.StyleDefault
		descStyle := tcell.StyleDefault
		iconStyle := tcell.StyleDefault
		if c, ok := hexToColour(item.Source.Foreground); ok {
			iconStyle = iconStyle.Foreground(c)
		}
		if c, ok := hexToColour(item.Source.Background); ok {
			iconStyle = iconStyle.Background(c)
		}

		badge := item.Source.Name
		if len(badge) > maxBadge {
			badge = badge[:maxBadge]
		}
		badgeOff := maxBadge - len(badge)

		switch {
		case !g.interacting:
			// everything should be coloured
			// colour the icon if needed
			_, bg, _ := iconStyle.Decompose()
			roundStyle := tcell.StyleDefault.Foreground(bg)
			g.printf(badgeOff, y, roundStyle, "%s", "")
			x := g.printf(badgeOff+1, y, iconStyle.Bold(true), "%s", badge)
			g.printf(x+badgeOff+1, y, roundStyle, "%s", "")
			g.printf(maxBadge+4, y, titleStyle, "%s", item.Title)
			if g.config.ShowDescriptions {
				g.printf(maxBadge+4, y+1, descStyle, "%s", item.Description)
			}
		case selected:
			// highlight selected
			selectStyle := iconStyle
			_, bg, _ := selectStyle.Decompose()
			roundStyle := tcell.StyleDefault.Foreground(bg)
			g.printf(badgeOff+1, y, descStyle.Bold(true), "%s", badge)
			g.printf(maxBadge+3, y, roundStyle, "%s", "")
			x := g.printf(maxBadge+4, y, selectStyle, "%s", item.Title)
			g.printf(x+maxBadge+4, y, roundStyle, "%s", "")
			if g.config.ShowDescriptions {
				g.printf(maxBadge+4, y+1, descStyle, "%s", item.Description)
			}
		default:
			// dim everything else
			dimStyle := tcell.StyleDefault.Dim(true).Foreground(tcell.NewRGBColor(190, 190, 190))
			g.printf(badgeOff+1, y, dimStyle, "%s", badge)
			g.printf(1+maxBadge+3, y, dimStyle, "%s", item.Title)
			if g.config.ShowDescriptions {
				g.printf(1+maxBadge+3, y+1, dimStyle, "%s", item.Description)
			}
		}

	}

	if g.filtering {
		if g.filterEditing {
			g.printf(0, h-1, tcell.StyleDefault, "Filter: %s█", g.filter)
		} else {
			g.printf(0, h-1, tcell.StyleDefault.Foreground(tcell.ColorLimeGreen), "Filter: %s (esc to clear)", g.filter)
		}
	} else if g.config.ShowHelp {
		remaining := (g.config.PollInterval - time.Since(g.lastUpdate)).Round(time.Second)
		when := "in " + remaining.String()
		if remaining < time.Second {
			when = "now"
		}
		g.printf(0, h-1, tcell.StyleDefault.Dim(true).Foreground(tcell.NewRGBColor(150, 150, 150)), "%s | updating %s", helpText, when)
	}

	g.screen.Show()
}

func (g *GUI) printf(x, y int, style tcell.Style, str string, attrs ...interface{}) int {
	str = fmt.Sprintf(str, attrs...)
	start := x
	for _, c := range str {
		var comb []rune
		w := runewidth.RuneWidth(c)
		if w == 0 {
			comb = []rune{c}
			c = ' '
			w = 1
		}
		g.screen.SetContent(x, y, c, comb, style)
		x += w
	}
	return x - start
}

func hexToColour(h string) (tcell.Color, bool) {
	if h != "" && h[0] == '#' {
		data, err := hex.DecodeString(h[1:])
		if err == nil && len(data) == 3 {
			return tcell.NewRGBColor(int32(data[0]), int32(data[1]), int32(data[2])), true
		}
	}
	return tcell.ColorDefault, false
}
