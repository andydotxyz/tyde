package ui

import (
	"image/color"
	"os"
	"sort"
	"time"

	"github.com/FyshOS/appie"

	_ "github.com/fyne-io/image/xpm" // load in unix image format

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	deskDriver "fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	wmtheme "fyshos.com/fynedesk/theme"
)

func (w *widgetPanel) appendAppCategories(acc *widget.Accordion, closer func()) {
	accList := acc.Items
	cats := w.desk.IconProvider().CategorizedApps()
	var catNames []string
	hasOther := false
	for cat := range cats {
		if cat == "Other" {
			hasOther = true
			continue
		}
		catNames = append(catNames, cat)
	}
	sort.Strings(catNames)
	if hasOther {
		catNames = append(catNames, "Other")
	}

	for _, cat := range catNames {
		list := cats[cat]
		var items []fyne.CanvasObject
		for _, app := range list {
			if app.Hidden() {
				continue
			}
			btn := w.newAppButton(app, closer)
			items = append(items, btn)
			defer w.loadIcon(app, btn)
		}
		accList = append(accList, widget.NewAccordionItem(cat,
			container.NewVBox(items...)))
	}

	fyne.Do(func() {
		acc.Items = accList
		acc.Refresh()
	})
}

func (w *widgetPanel) askLogout() {
	win := fyne.CurrentApp().Driver().(deskDriver.Driver).CreateSplashWindow()
	logout := widget.NewButtonWithIcon("Logout", theme.LogoutIcon(), func() {
		win.Close()
		time.Sleep(time.Second / 10)
		w.desk.WindowManager().Close()
	})
	logout.Importance = widget.DangerImportance
	cancel := widget.NewButton("Cancel", func() {
		win.Close()
	})

	header := widget.NewRichTextFromMarkdown("### Log out")
	header.Truncation = fyne.TextTruncateEllipsis
	bottomPad := canvas.NewRectangle(color.Transparent)
	bottomPad.SetMinSize(fyne.NewSquareSize(10))
	content := container.NewBorder(
		header,
		container.NewVBox(
			container.NewHBox(layout.NewSpacer(),
				container.NewGridWithColumns(2, cancel, logout),
				layout.NewSpacer()), bottomPad),
		nil, nil,
		widget.NewLabel("Are you sure you want to log out?"))

	r, g, b, _ := theme.Color(theme.ColorNameOverlayBackground).RGBA()
	bgCol := &color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 230}

	bg := canvas.NewRectangle(bgCol)
	icon := canvas.NewImageFromResource(theme.LogoutIcon())
	iconBox := container.NewWithoutLayout(icon)
	icon.Resize(fyne.NewSize(92, 92))
	icon.Move(fyne.NewPos(280-92-theme.Padding(), theme.Padding()))
	win.SetContent(container.NewStack(
		iconBox, bg,
		container.NewPadded(content)))

	win.Resize(fyne.NewSize(280, 150))
	win.CenterOnScreen()
	win.Show()
}

func (w *widgetPanel) showAccountMenu(_ fyne.CanvasObject) {
	var closer func()
	items1 := []fyne.CanvasObject{
		&widget.Button{Icon: theme.LogoutIcon(), Importance: widget.DangerImportance, OnTapped: func() {
			w.askLogout()
			closer()
		}}}
	isEmbed := w.desk.(*desktop).root.Title() != RootWindowName
	items1 = append(items1, &widget.Button{Icon: wmtheme.LockIcon, Importance: widget.LowImportance, OnTapped: func() {
		closer()
		w.desk.TriggerScreenSaver(false)
	}})
	if !isEmbed {
		if os.Getenv("FYNE_DESK_RUNNER") != "" {
			items1 = append(items1, &widget.Button{Icon: theme.ViewRefreshIcon(), Importance: widget.LowImportance, OnTapped: func() {
				os.Exit(5)
			}})
		}
	}

	items2 := []fyne.CanvasObject{
		&widget.Button{Icon: theme.QuestionIcon(), Importance: widget.LowImportance, OnTapped: func() {
			w.showAbout()
			closer()
		}},
		&widget.Button{Icon: theme.SettingsIcon(), Importance: widget.LowImportance, OnTapped: func() {
			w.showSettings()
			closer()
		}}}
	items := container.NewBorder(nil, nil, container.NewHBox(items1...), container.NewHBox(items2...),
		&widget.Button{Icon: theme.SearchIcon(), Text: "Search", Importance: widget.LowImportance, OnTapped: func() {
			ShowAppLauncher()
			closer()
		}})

	var recent []fyne.CanvasObject
	for _, app := range w.desk.RecentApps() {
		btn := w.newAppButton(app, func() {
			closer() // not initialised on creation
		})
		recent = append(recent, btn)

		btn.Icon = app.Icon(w.desk.Settings().IconTheme(), int(64*w.desk.Screens().Primary().CanvasScale()))
	}

	acc := widget.NewAccordion(widget.NewAccordionItem("Recent",
		container.NewVBox(recent...)))
	acc.MultiOpen = true
	acc.Open(0)

	content := container.NewPadded(container.NewBorder(
		items, nil, nil, nil,
		container.NewScroll(acc)))
	winSize := w.desk.(*desktop).root.Canvas().Size()
	pos := fyne.NewPos(winSize.Width-300, winSize.Height-360)
	_, closer = w.desk.WindowManager().ShowOverlay(content, nil, fyne.NewSize(300, 360), pos)
	go w.appendAppCategories(acc, closer)
}

func (w *widgetPanel) newAppButton(app appie.AppData, closer func()) *widget.Button {
	b := widget.NewButtonWithIcon(app.Name(), wmtheme.BrokenImageIcon, func() {
		closer()
		_ = w.desk.RunApp(app)
	})
	b.Alignment = widget.ButtonAlignLeading
	return b
}

func (w *widgetPanel) loadIcon(app appie.AppData, btn *widget.Button) {
	iconRes := app.Icon(w.desk.Settings().IconTheme(), int(64*w.desk.Screens().Primary().CanvasScale()))

	fyne.Do(func() {
		btn.SetIcon(iconRes)
	})
}
