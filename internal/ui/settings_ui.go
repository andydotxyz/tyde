package ui

import (
	"embed"
	"fmt"
	"image/color"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	deskDriver "fyne.io/fyne/v2/driver/desktop"
	"github.com/FyshOS/appie"
	"github.com/FyshOS/backgrounds"
	"github.com/FyshOS/screens/pkg/screenmanager"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/cmd/fyne_settings/settings"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"fyshos.com/tyde"
	wmtheme "fyshos.com/tyde/theme"
	"fyshos.com/tyde/wm"
)

//go:embed "themes/*"
var bundledThemes embed.FS

type settingsUI struct {
	settings *deskSettings
	win      fyne.Window

	fyneSettings  *settings.Settings
	launcherIcons []string
}

func (d *settingsUI) populateThemeIcons(box *fyne.Container, theme string) {
	box.Objects = nil
	for _, appName := range d.launcherIcons {
		appData := tyde.Instance().IconProvider().FindAppFromName(appName)
		if appData == nil { // if app was removed!
			continue
		}
		iconRes := appData.Icon(theme, int(32*tyde.Instance().Screens().Primary().CanvasScale()))
		icon := widget.NewIcon(iconRes)
		box.Add(icon)
	}
	box.Refresh()
}

func (d *settingsUI) loadAppearanceScreen() fyne.CanvasObject {
	clockLabel := widget.NewLabelWithStyle("Clock Format", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	clockFormat := &widget.RadioGroup{Options: []string{"12h", "24h"}, Required: true, Horizontal: true}
	clockFormat.SetSelected(d.settings.ClockFormatting())

	layoutLabel := widget.NewLabelWithStyle("Layout", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	narrowWidget := widget.NewCheck("Narrow Widget Bar", nil)
	narrowWidget.Checked = d.settings.NarrowWidgetPanel()

	borderButtonLabel := widget.NewLabelWithStyle("Border Button Position", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	borderButton := &widget.Select{Options: []string{"Left", "Right"}}
	borderButton.SetSelected(d.settings.BorderButtonPosition())

	saverLabel := widget.NewLabelWithStyle("Screensaver", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	saverType := &widget.RadioGroup{Options: []string{"FyshOS", "XScreensaver"}, Required: true, Horizontal: true}
	saverType.SetSelected(d.settings.ScreenSaverType())
	saverText := widget.NewEntry()
	saverText.SetText(d.settings.ScreenSaverLabel())
	saverClock := widget.NewCheck("Clock", nil)
	saverClock.Checked = d.settings.ScreenSaverClock()

	themeLabel := widget.NewLabel(d.settings.IconTheme())
	themeIcons := container.NewHBox()
	d.populateThemeIcons(themeIcons, d.settings.IconTheme())
	themeList := container.NewVBox()
	for _, themeName := range tyde.Instance().IconProvider().AvailableThemes() {
		themeButton := widget.NewButton(themeName, nil)
		themeButton.OnTapped = func() {
			themeLabel.SetText(themeButton.Text)

			tyde.Instance().IconProvider().ClearCache()
			d.populateThemeIcons(themeIcons, themeButton.Text)
		}
		themeList.Add(themeButton)
	}

	time := container.NewBorder(nil, nil, clockLabel, clockFormat)
	lay := container.NewBorder(nil, nil, layoutLabel, narrowWidget)
	border := container.NewBorder(nil, nil, borderButtonLabel, borderButton)
	saver := container.NewBorder(nil, nil, container.NewVBox(saverLabel, widget.NewLabel("")),
		container.NewVBox(saverType, container.NewBorder(nil, nil, saverClock, nil, saverText)))
	top := container.NewVBox(time, lay, border, saver)

	themeFormLabel := widget.NewLabelWithStyle("Icon Theme", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	themeCurrent := container.NewHBox(layout.NewSpacer(), themeLabel, themeIcons)
	bottom := container.NewBorder(nil, themeCurrent, themeFormLabel, nil, container.NewScroll(themeList))

	applyButton := container.NewHBox(layout.NewSpacer(),
		&widget.Button{Text: "Apply", Importance: widget.HighImportance, OnTapped: func() {
			d.settings.setIconTheme(themeLabel.Text)
			d.settings.setClockFormatting(clockFormat.Selected)
			d.settings.setBorderButtonPosition(borderButton.Selected)
			d.settings.setNarrowWidgetPanel(narrowWidget.Checked)
			d.settings.setScreenSaver(saverType.Selected)
			d.settings.setScreenSaverClock(saverClock.Checked)
			d.settings.setScreenSaverLabel(saverText.Text)
		}})

	return container.NewBorder(top, applyButton, nil, nil, bottom)
}

func (d *settingsUI) loadBackgroundScreen() fyne.CanvasObject {
	var bgPathClear *widget.Button
	bgPath := widget.NewEntry()
	bgPath.SetPlaceHolder("Choose an image")
	bgPathClear = widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
		bgPath.SetText("")
		bgPathClear.Disable()
	})

	if fyne.CurrentApp().Preferences().String("background") != "" {
		bgPath.SetText(fyne.CurrentApp().Preferences().String("background"))
	} else {
		bgPathClear.Disable()
	}

	bgDialog := dialog.NewFileOpen(func(file fyne.URIReadCloser, err error) {
		if err != nil || file == nil {
			return
		}

		// not advisable for cross-platform but we are desktop only
		path := file.URI().String()[7:]
		_ = file.Close()

		bgPath.SetText(path)
		bgPathClear.Enable()
	}, d.win)
	bgDialog.SetFilter(storage.NewExtensionFileFilter([]string{".jpg", ".jpeg", ".png", ".svg"}))
	if dir, err := getPicturesDir(); err == nil {
		bgDialog.SetLocation(dir)
	} else {
		fyne.LogError("error finding pictures dir, falling back to home directory", err)
	}

	bgButtons := container.NewHBox(bgPathClear,
		widget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() {
			bgDialog.Show()
		}))

	// Live preview of the chosen image, rendered inside a monitor surround.
	screen := canvas.NewImageFromFile("")
	screen.ScaleMode = canvas.ImageScaleFastest

	// Solid colour drawn behind the image, shown wherever it does not cover.
	screenColor := canvas.NewRectangle(ParseHexColor(d.settings.BackgroundColor()))
	preview := container.NewCenter(monitorSurround(screen, screenColor))

	// The default wallpaper used by the desktop when no image is configured.
	set := fyne.CurrentApp().Settings()
	defaultBg, _ := backgrounds.Default().Load(set.Theme(), set.ThemeVariant()).(*canvas.Image)

	fillSelect := widget.NewSelect(backgroundFillModes, nil)

	refreshPreview := func() {
		if bgPath.Text == "" {
			// No image set: mirror the desktop's default wallpaper, which
			// always covers the screen and ignores the fill/colour options.
			screen.File = ""
			if defaultBg != nil {
				screen.Resource = defaultBg.Resource
			}
			screen.FillMode = canvas.ImageFillCover
		} else {
			screen.Resource = nil
			screen.File = bgPath.Text
			screen.FillMode = backgroundFillMode(fillSelect.Selected)
		}
		screen.Refresh()
	}

	fillSelect.OnChanged = func(string) { refreshPreview() }
	fillSelect.SetSelected(d.settings.BackgroundFill())
	bgPath.OnChanged = func(string) { refreshPreview() }
	refreshPreview() // initialise from the current setting

	// A small swatch showing the currently selected background colour.
	colorSwatch := canvas.NewRectangle(screenColor.FillColor)
	colorSwatch.CornerRadius = theme.Size(theme.SizeNameInputRadius)
	colorSwatch.SetMinSize(fyne.NewSize(24, 24))

	colorButton := widget.NewButtonWithIcon("Colour", theme.ColorChromaticIcon(), func() {
		picker := dialog.NewColorPicker("Background colour", "Colour drawn behind the image",
			func(c color.Color) {
				screenColor.FillColor = c
				screenColor.Refresh()
				colorSwatch.FillColor = c
				colorSwatch.Refresh()
			}, d.win)
		picker.Advanced = true
		picker.SetColor(screenColor.FillColor)
		picker.Show()
	})
	colorControls := container.NewHBox(container.NewCenter(colorSwatch), colorButton)
	fillRow := container.NewBorder(nil, nil, widget.NewLabel("Fill"), colorControls, fillSelect)

	applyButton := container.NewHBox(layout.NewSpacer(),
		&widget.Button{Text: "Apply", Importance: widget.HighImportance, OnTapped: func() {
			d.settings.setBackground(bgPath.Text)
			d.settings.setBackgroundFill(fillSelect.Selected)
			d.settings.setBackgroundColor(HexColor(screenColor.FillColor))
		}})

	return container.NewBorder(nil, applyButton, nil, nil,
		widget.NewCard("Background", "",
			container.NewBorder(
				container.NewVBox(
					container.NewBorder(nil, nil, nil, bgButtons, bgPath),
					fillRow,
				),
				nil, nil, nil, preview,
			)))
}

// monitorSurround wraps the given screen image in a simple monitor-shaped frame:
// a dark bezel around the screen area sitting on a small stand. The screen area
// matches the aspect ratio of the primary display so the preview is faithful.
func monitorSurround(screen *canvas.Image, screenColor *canvas.Rectangle) fyne.CanvasObject {
	frameColor := color.NRGBA{R: 0x2b, G: 0x2b, B: 0x2b, A: 0xff}

	const previewHeight = 144
	previewWidth := float32(previewHeight) * 16.0 / 9.0 // default 16:9
	if screens := tyde.Instance().Screens(); screens != nil {
		if primary := screens.Primary(); primary != nil && primary.Height > 0 {
			previewWidth = float32(previewHeight) * float32(primary.Width) / float32(primary.Height)
		}
	}
	screen.SetMinSize(fyne.NewSize(previewWidth, previewHeight))

	bezel := canvas.NewRectangle(frameColor)
	bezel.CornerRadius = theme.Size(theme.SizeNameInputRadius)
	display := container.NewStack(bezel, container.NewPadded(container.NewStack(screenColor, screen)))

	neck := canvas.NewRectangle(frameColor)
	neck.SetMinSize(fyne.NewSize(28, 14))
	base := canvas.NewRectangle(frameColor)
	base.CornerRadius = theme.Size(theme.SizeNameInputRadius)
	base.SetMinSize(fyne.NewSize(96, 8))
	stand := container.NewVBox(container.NewCenter(neck), container.NewCenter(base))

	return container.NewVBox(display, stand)
}

func (d *settingsUI) populateOrderList(list *fyne.Container, add fyne.CanvasObject) {
	var icons []fyne.CanvasObject
	for i, appName := range d.launcherIcons {
		index := i // capture
		appData := tyde.Instance().IconProvider().FindAppFromName(appName)
		if appData == nil {
			continue // uninstalled?
		}
		left := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() {
			d.launcherIcons[index-1], d.launcherIcons[index] = d.launcherIcons[index], d.launcherIcons[index-1]
			d.populateOrderList(list, add)
		})
		if index <= 0 {
			left.Disable()
		}

		remove := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
			if index == 0 {
				d.launcherIcons = d.launcherIcons[1:]
			} else if index == len(d.launcherIcons)-1 {
				d.launcherIcons = d.launcherIcons[:len(d.launcherIcons)-1]
			} else {
				d.launcherIcons = append(d.launcherIcons[:index], d.launcherIcons[index+1])
			}
			d.populateOrderList(list, add)
		})

		right := widget.NewButtonWithIcon("", theme.NavigateNextIcon(), func() {
			d.launcherIcons[index+1], d.launcherIcons[index] = d.launcherIcons[index], d.launcherIcons[index+1]
			d.populateOrderList(list, add)
		})
		if index >= len(d.launcherIcons)-1 {
			right.Disable()
		}
		iconRes := appData.Icon(d.settings.IconTheme(), int(32*tyde.Instance().Screens().Primary().CanvasScale()))
		icon := canvas.NewImageFromResource(iconRes)
		icon.FillMode = canvas.ImageFillContain
		icon.SetMinSize(fyne.NewSquareSize(32))
		label := widget.NewLabelWithStyle(appName, fyne.TextAlignCenter, fyne.TextStyle{})
		hbox := container.NewVBox(icon, label, container.NewHBox(left, remove, right))
		icons = append(icons, hbox)
	}

	icons = append(icons, add)
	list.Objects = icons
	list.Refresh()
}

func (d *settingsUI) loadBarScreen() fyne.CanvasObject {
	addButton := widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {})
	addIcon := canvas.NewImageFromResource(theme.ContentAddIcon())
	addIcon.FillMode = canvas.ImageFillContain
	addIcon.SetMinSize(fyne.NewSquareSize(32))
	addItem := container.NewVBox(addIcon, widget.NewLabel("Add Icon"), addButton)
	orderList := container.NewHBox()
	d.populateOrderList(orderList, addItem)

	addButton.OnTapped = func() {
		p := newAppPicker(func(data appie.AppData, _ int) {
			d.launcherIcons = append(d.launcherIcons, data.Name())
			d.populateOrderList(orderList, addItem)
		})
		p.show()
	}

	bar := container.NewHScroll(orderList)

	disableTaskbar := widget.NewCheck("Disable Taskbar", nil)
	disableTaskbar.SetChecked(d.settings.LauncherDisableTaskbar())

	details := container.NewVBox(widget.NewRichTextFromMarkdown("# Configuration"),
		disableTaskbar)

	applyButton := container.NewHBox(layout.NewSpacer(),
		&widget.Button{Text: "Apply", Importance: widget.HighImportance, OnTapped: func() {
			d.settings.setLauncherDisableTaskbar(disableTaskbar.Checked)
			d.settings.setLauncherIcons(d.launcherIcons)
		}})

	return container.NewBorder(nil, applyButton, nil, nil,
		widget.NewCard("App Bar", "", container.NewVBox(bar, details)))
}

func (d *settingsUI) loadModulesScreen() fyne.CanvasObject {
	var modules, launchers []fyne.CanvasObject

	applyModules := func() {
		var names []string
		for _, item := range modules {
			check := item.(*widget.Check)
			if check.Checked {
				names = append(names, check.Text)
			}
		}
		for _, item := range launchers {
			check := item.(*widget.Check)
			if check.Checked {
				names = append(names, "Launcher:"+check.Text)
			}
		}

		d.settings.setModuleNames(names)
	}

	for _, mod := range tyde.AvailableModules() {
		name := mod.Name
		enabled := isModuleEnabled(name, d.settings)

		check := widget.NewCheck(name, func(bool) {})
		check.SetChecked(enabled)
		check.OnChanged = func(_ bool) {
			applyModules()
		}

		if strings.Index(name, "Launcher:") == 0 {
			check.SetText(name[9:])
			launchers = append(launchers, check)
		} else {
			modules = append(modules, check)
		}
	}
	content := container.NewGridWithColumns(2,
		widget.NewCard("Modules", "",
			container.NewVScroll(container.NewVBox(modules...))),
		widget.NewCard("Launchers", "",
			container.NewVScroll(container.NewVBox(launchers...))))

	return content
}

func (d *settingsUI) loadKeyboardScreen() fyne.CanvasObject {
	var names, mods, keys []fyne.CanvasObject
	shortcuts := tyde.Instance().(wm.ShortcutManager).Shortcuts()
	sort.Slice(shortcuts, func(i, j int) bool {
		return strings.Compare(shortcuts[i].ShortcutName(), shortcuts[j].ShortcutName()) < 0
	})

	for _, shortcut := range shortcuts {
		names = append(names, widget.NewLabel(shortcut.ShortcutName()))
		mods = append(mods, widget.NewLabel(modifierToString(shortcut.Modifier, d.settings.modifier)))
		keys = append(keys, widget.NewLabel(string(shortcut.KeyName)))
	}
	modVBox := container.NewVBox(mods...)
	rows := container.NewHBox(widget.NewCard("Action", "", container.NewVBox(names...)),
		widget.NewCard("Modifier", "", modVBox),
		widget.NewCard("Key Name", "", container.NewVBox(keys...)))
	grid := container.NewScroll(rows)

	userMod := d.settings.modifier
	modType := widget.NewRadioGroup([]string{"Super", "Alt"}, func(mod string) {
		if mod == "Alt" {
			userMod = fyne.KeyModifierAlt
		} else {
			userMod = fyne.KeyModifierSuper
		}

		var mods []fyne.CanvasObject
		for _, shortcut := range shortcuts {
			mods = append(mods, widget.NewLabel(modifierToString(shortcut.Modifier, userMod)))
		}
		modVBox.Objects = mods
		modVBox.Refresh()

		d.settings.setKeyboardModifier(userMod)
	})
	modType.Horizontal = true
	if d.settings.modifier == fyne.KeyModifierAlt {
		modType.Selected = "Alt"
	} else {
		modType.Selected = "Super"
	}

	return container.NewBorder(
		widget.NewCard("Keyboard", "", container.NewHBox(widget.NewLabel("Preferred modifier key: "), modType)),
		nil, nil, nil, grid,
	)
}

func (d *settingsUI) loadThemeScreen() fyne.CanvasObject {
	var themeList []string

	embedList, _ := bundledThemes.ReadDir("themes")
	currentTheme := fyne.CurrentApp().Preferences().StringWithFallback("currentTheme", "default")
	for _, dir := range embedList {
		themeList = append(themeList, dir.Name())
	}

	storageRoot := fyne.CurrentApp().Storage().RootURI()
	themes, _ := storage.Child(storageRoot, "themes")
	list, err := storage.List(themes)
	if err != nil {
		fyne.LogError("Unable to list themes - missing?", err)
	} else {
		for _, l := range list {
			if false { // TODO with 1.21 } !slices.Contains(themeList, l.Name()) {
				themeList = append(themeList, l.Name())
			}
		}
	}

	useTheme := func(name string) {
		dest := filepath.Join(filepath.Dir(storageRoot.Path()), "theme.json")
		out, _ := os.Create(dest)
		defer out.Close()
		if name == "default" {
			_, _ = io.WriteString(out, "{}")
			return
		}

		var in io.ReadCloser
		if builtin, err := bundledThemes.Open(filepath.Join("themes/", name, "theme.json")); err == nil {
			in = builtin
		} else {
			source := filepath.Join(themes.Path(), name, "theme.json")
			in, _ = os.Open(source)
		}
		defer in.Close()

		_, err = io.Copy(out, in)
	}
	var themesWidget *widget.List
	themesWidget = widget.NewList(
		func() int {
			return len(themeList)
		},
		func() fyne.CanvasObject {
			install := widget.NewButtonWithIcon("Install", theme.ComputerIcon(), nil)
			preview := &canvas.Image{FillMode: canvas.ImageFillContain}
			preview.SetMinSize(fyne.NewSize(160, 90))
			return container.NewBorder(nil, nil, nil, preview,
				container.NewBorder(nil, install, nil, nil,
					widget.NewRichTextFromMarkdown("## Theme Name\n\nDescription...")))
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			outer := o.(*fyne.Container)
			inner := outer.Objects[0].(*fyne.Container)
			b := inner.Objects[1].(*widget.Button)
			themeName := themeList[id]
			if themeName == currentTheme {
				b.Disable()
			} else {
				b.Enable()
			}

			b.OnTapped = func() {
				currentTheme = themeName
				fyne.CurrentApp().Preferences().SetString("currentTheme", themeName)
				themesWidget.Refresh()

				useTheme(themeName)
			}
			p := outer.Objects[1].(*canvas.Image)
			if builtin, err := bundledThemes.Open(filepath.Join("themes/", themeList[id], "preview.png")); err == nil {
				data, _ := io.ReadAll(builtin)
				p.Resource = fyne.NewStaticResource(themeList[id]+"/preview.json", data)
				p.File = ""
				_ = builtin.Close()
			} else {
				source := filepath.Join(themes.Path(), themeList[id], "preview.png")
				p.File = source
				p.Resource = nil
			}
			p.Refresh()

			l := inner.Objects[0].(*widget.RichText)
			title := cases.Title(language.Make("en")).String(themeList[id])
			l.ParseMarkdown(fmt.Sprintf("## %s\n\nDescription...", title))
		},
	)

	addNew := widget.NewButton("More themes...", func() {
		u, _ := url.Parse("https://fyshos.com/themes")
		_ = fyne.CurrentApp().OpenURL(u)
	})

	custom := container.NewHBox(layout.NewSpacer(), widget.NewButton("Customise...", d.showCustomise))
	return container.NewBorder(nil, custom, nil, nil,
		widget.NewCard("Themes", "", container.NewBorder(nil, addNew, nil, nil, themesWidget)))
}

func (w *widgetPanel) showSettings() {
	if w.settings != nil {
		w.settings.CenterOnScreen()
		w.settings.Show()
		w.settings.(deskDriver.Window).RequestAlwaysOnTop()
		return
	}

	deskSettings := w.desk.Settings().(*deskSettings)
	ui := &settingsUI{
		settings:      deskSettings,
		launcherIcons: deskSettings.LauncherIcons(),
	}

	win := fyne.CurrentApp().NewWindow("Tyde Settings")
	ui.win = win

	scale := ui.makeScaleGroup(win)
	screens := screenmanager.New(win)
	screens.OnConfigurationChanged = w.desk.Screens().RefreshScreens
	screenui := widget.NewCard("Screens", "", screens)
	win.SetOnClosed(screens.Close)

	tabs := container.NewAppTabs(
		&container.TabItem{
			Text: "Appearance", Icon: ui.fyneSettings.AppearanceIcon(),
			Content: ui.loadAppearanceScreen(),
		},
		&container.TabItem{
			Text: "Background", Icon: wmtheme.WallpaperIcon,
			Content: ui.loadBackgroundScreen(),
		},
		&container.TabItem{Text: "App Bar", Icon: wmtheme.IconifyIcon, Content: ui.loadBarScreen()},
		&container.TabItem{
			Text: "Display", Icon: wmtheme.ScreensIcon,
			Content: container.NewBorder(scale, nil, nil, nil, screenui),
		},
		&container.TabItem{Text: "Time/Date", Icon: wmtheme.ClockIcon, Content: ui.loadTimeScreen()},
		&container.TabItem{Text: "Theme", Icon: theme.ColorPaletteIcon(), Content: ui.loadThemeScreen()},
		&container.TabItem{Text: "Keyboard", Icon: wmtheme.KeyboardIcon, Content: ui.loadKeyboardScreen()},
		&container.TabItem{
			Text: "Modules", Icon: theme.SettingsIcon(),
			Content: ui.loadModulesScreen(),
		},
	)
	tabs.SetTabLocation(container.TabLocationLeading)

	// FyshOS logo watermark in the bottom-left, behind the tab icons and no
	// wider than the tab bar (see tabLogoLayout).
	logo := canvas.NewImageFromResource(wmtheme.LogoFade)
	logo.Translucency = 0.4
	logo.SetMinSize(fyne.NewSquareSize(barWidth(tabs.Items)))
	win.SetContent(container.NewStack(
		container.NewVBox(layout.NewSpacer(), container.NewHBox(logo)),
		tabs,
	))
	win.Resize(fyne.NewSize(480, 320))

	win.SetCloseIntercept(func() {
		win.Hide()
	})
	w.settings = win
	win.Show()
}

// barWidth mirrors Fyne's leading tab-bar sizing, returning the tab width from AppTabs.
func barWidth(tabs []*container.TabItem) float32 {
	iconSize := 1.5 * theme.Size(theme.SizeNameInlineIcon)
	textSize := theme.Size(theme.SizeNameText)
	innerPad := theme.Size(theme.SizeNameInnerPadding)

	maxW := float32(0)
	for _, it := range tabs {
		w := fyne.Max(fyne.MeasureText(it.Text, textSize, fyne.TextStyle{}).Width, iconSize) + innerPad
		maxW = fyne.Max(maxW, w)
	}
	return maxW
}

func modifierToString(mods fyne.KeyModifier, userMod fyne.KeyModifier) string {
	var s []string
	if (mods & tyde.UserModifier) != 0 {
		mods |= userMod
	}

	if (mods & fyne.KeyModifierShift) != 0 {
		s = append(s, "Shift")
	}
	if (mods & fyne.KeyModifierControl) != 0 {
		s = append(s, "Control")
	}
	if (mods & fyne.KeyModifierAlt) != 0 {
		s = append(s, "Alt")
	}
	if (mods & fyne.KeyModifierSuper) != 0 {
		if runtime.GOOS == "darwin" {
			s = append(s, "Command")
		} else {
			s = append(s, "Super")
		}
	}
	return strings.Join(s, "+")
}

func getPicturesDir() (fyne.ListableURI, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	const xdg = "xdg-user-dir"
	if _, err := exec.LookPath(xdg); err == nil {
		cmd := exec.Command(xdg, "PICTURES")

		out, err := cmd.Output()
		location := string(out[:len(out)-1]) // Remove \n at the end
		if err == nil && location != home {
			uri := storage.NewFileURI(location)
			return storage.ListerForURI(uri)
		}
	}

	uri, err := storage.Child(storage.NewFileURI(home), "Pictures")
	if err != nil {
		return nil, err
	}

	return storage.ListerForURI(uri)
}

func (d *settingsUI) makeScaleGroup(w fyne.Window) fyne.CanvasObject {
	s := settings.NewSettings()
	fyneAppearance := s.LoadAppearanceScreen(w)

	preview := fyneAppearance.(*fyne.Container).Objects[0]
	preview.Hide()
	box := fyneAppearance.(*fyne.Container).Objects[1]
	box.(*fyne.Container).Objects[1].Hide() // appearance card

	applyRow := fyneAppearance.(*fyne.Container).Objects[2].(*fyne.Container)
	submit := applyRow.Objects[1].(*widget.Button)
	applyRow.Hide()

	scale := box.(*fyne.Container).Objects[0].(*widget.Card)
	buttons := scale.Content.(*fyne.Container).Objects[1].(*fyne.Container).Objects

	for _, b := range buttons {
		tap := b.(*widget.Button).OnTapped
		b.(*widget.Button).OnTapped = func() {
			tap()
			submit.OnTapped()
		}
	}
	return fyneAppearance
}

func (d *settingsUI) showCustomise() {
	s := settings.NewSettings()
	w := fyne.CurrentApp().NewWindow("Customise Theme")
	fyneAppearance := s.LoadAppearanceScreen(w)

	box := fyneAppearance.(*fyne.Container).Objects[1]
	box.(*fyne.Container).Objects[0].Hide() // scale card

	appearance := box.(*fyne.Container).Objects[1].(*widget.Card)
	appearance.SetTitle("Customise Theme")

	w.SetContent(fyneAppearance)
	w.Show()
}
