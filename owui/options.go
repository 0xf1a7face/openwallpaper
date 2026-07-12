package main

import (
	"os/exec"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func buildOptionsPage(state *appState) *gtk.ScrolledWindow {
	content := gtk.NewBox(gtk.OrientationVertical, 12)
	content.SetMarginTop(18)
	content.SetMarginBottom(18)
	content.SetMarginStart(18)
	content.SetMarginEnd(18)
	content.SetHExpand(true)
	content.SetVAlign(gtk.AlignStart)
	content.SetHAlign(gtk.AlignFill)

	clamp := adw.NewClamp()
	clamp.SetHExpand(true)
	clamp.SetMaximumSize(optionsMaxWidth)
	clamp.SetTighteningThreshold(optionsMaxWidth)
	clamp.SetChild(content)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetHExpand(true)
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scrolled.SetChild(clamp)

	addGeneralOptions(content, state)
	addSceneOptions(content, state)
	addVideoOptions(content, state)

	return scrolled
}

func addGeneralOptions(content *gtk.Box, state *appState) {
	content.Append(sectionLabel("General"))

	list := boxedList()

	hiddenSwitch := gtk.NewSwitch()
	hiddenSwitch.SetActive(state.settings.PauseHidden)
	hiddenSwitch.SetVAlign(gtk.AlignCenter)
	list.Append(labeledWidgetRow("Pause when wallpaper is hidden", hiddenSwitch))
	bindSwitch(hiddenSwitch, state, func(s *settings, value bool) {
		s.PauseHidden = value
	})

	batterySwitch := gtk.NewSwitch()
	batterySwitch.SetActive(state.settings.PauseOnBat)
	batterySwitch.SetVAlign(gtk.AlignCenter)
	list.Append(labeledWidgetRow("Pause when laptop is on battery power", batterySwitch))
	bindSwitch(batterySwitch, state, func(s *settings, value bool) {
		s.PauseOnBat = value
	})

	steamLibrary := gtk.NewEntry()
	steamLibrary.SetPlaceholderText(defaultSteamLibraryPath())
	steamLibrary.SetText(state.settings.SteamLibraryPath)
	list.Append(entryRow("Steam Library path", steamLibrary))
	bindEntry(steamLibrary, state, func(s *settings, value string) {
		s.SteamLibraryPath = value
	})

	content.Append(list)
}

func addSceneOptions(content *gtk.Box, state *appState) {
	content.Append(sectionLabel("Scene"))

	list := boxedList()

	vSyncSwitch := gtk.NewSwitch()
	vSyncSwitch.SetActive(state.settings.VSync)
	vSyncSwitch.SetVAlign(gtk.AlignCenter)
	list.Append(labeledWidgetRow("V-Sync", vSyncSwitch))

	fpsSpin := gtk.NewSpinButtonWithRange(1, 720, 1)
	fpsSpin.SetValue(float64(state.settings.FPSLimit))
	fpsSpin.SetSensitive(!state.settings.VSync)
	list.Append(labeledWidgetRow("FPS limit", fpsSpin))

	dgpuSwitch := gtk.NewSwitch()
	dgpuSwitch.SetActive(state.settings.PreferDiscreteGPU)
	dgpuSwitch.SetVAlign(gtk.AlignCenter)
	list.Append(labeledWidgetRow("Prefer discrete GPU", dgpuSwitch))
	bindSwitch(dgpuSwitch, state, func(s *settings, value bool) {
		s.PreferDiscreteGPU = value
	})

	vSyncSwitch.ConnectStateSet(func(value bool) bool {
		state.settings.VSync = value
		fpsSpin.SetSensitive(!value)
		saveSettings(*state.settings)
		return false
	})

	fpsSpin.ConnectValueChanged(func() {
		state.settings.FPSLimit = fpsSpin.ValueAsInt()
		saveSettings(*state.settings)
	})

	audioSwitch := gtk.NewSwitch()
	audioSwitch.SetActive(state.settings.AudioVisualization)
	audioSwitch.SetVAlign(gtk.AlignCenter)
	list.Append(labeledWidgetRow("Enable audio visualization", audioSwitch))

	backendDropdown := gtk.NewDropDownFromStrings([]string{"Default", "PipeWire", "PulseAudio", "PortAudio"})
	backendDropdown.SetSelected(min(state.settings.AudioBackend, 3))
	backendDropdown.SetSensitive(state.settings.AudioVisualization)
	list.Append(labeledWidgetRow("Audio backend", backendDropdown))

	audioSource := gtk.NewEntry()
	audioSource.SetPlaceholderText("Default")
	audioSource.SetText(state.settings.AudioSource)
	audioSource.SetSensitive(state.settings.AudioVisualization)
	list.Append(entryRow("Audio source", audioSource))

	audioSwitch.ConnectStateSet(func(value bool) bool {
		state.settings.AudioVisualization = value
		saveSettings(*state.settings)
		backendDropdown.SetSensitive(value)
		audioSource.SetSensitive(value)
		return false
	})

	backendDropdown.NotifyProperty("selected", func() {
		state.settings.AudioBackend = backendDropdown.Selected()
		saveSettings(*state.settings)
	})

	bindEntry(audioSource, state, func(s *settings, value string) {
		s.AudioSource = value
	})

	content.Append(list)
}

func addVideoOptions(content *gtk.Box, state *appState) {
	content.Append(sectionLabel("Video"))

	list := boxedList()

	scaleModeDropdown := gtk.NewDropDownFromStrings([]string{"Aspect crop", "Aspect fit", "Stretch"})
	scaleModeDropdown.SetSelected(scaleModeIndex(state.settings.VideoScaleMode))
	list.Append(labeledWidgetRow("Scale mode", scaleModeDropdown))
	scaleModeDropdown.NotifyProperty("selected", func() {
		state.settings.VideoScaleMode = scaleModeValue(scaleModeDropdown.Selected())
		saveSettings(*state.settings)
	})

	filterOptions := overrideFilterOptions(state.settings.VideoFilter)
	filterDropdown := gtk.NewDropDownFromStrings(filterOptions)
	filterDropdown.SetSelected(filterOptionIndex(filterOptions, state.settings.VideoFilter))
	list.Append(labeledWidgetRow("Filter", filterDropdown))

	filterDropdown.NotifyProperty("selected", func() {
		selected := filterDropdown.Selected()
		if selected == 0 || int(selected) >= len(filterOptions) {
			state.settings.VideoFilter = ""
		} else {
			state.settings.VideoFilter = filterOptions[selected]
		}
		saveSettings(*state.settings)
	})

	content.Append(list)
}

func scaleModeIndex(value string) uint {
	switch normalizedScaleMode(value) {
	case "aspect-fit":
		return 1
	case "stretch":
		return 2
	default:
		return 0
	}
}

func scaleModeValue(index uint) string {
	switch index {
	case 1:
		return "aspect-fit"
	case 2:
		return "stretch"
	default:
		return "aspect-crop"
	}
}

func normalizedScaleMode(value string) string {
	switch value {
	case "aspect-fit", "stretch":
		return value
	default:
		return "aspect-crop"
	}
}

func loadMPVScaleFilters() []string {
	output, err := exec.Command("mpv", "--scale=help").CombinedOutput()
	if err != nil {
		return []string{"bilinear"}
	}
	filters := parseMPVScaleFilters(string(output))
	if len(filters) == 0 {
		return []string{"bilinear"}
	}
	return filters
}

func parseMPVScaleFilters(output string) []string {
	filters := []string{}
	seen := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		filter := strings.TrimSpace(line)
		if filter == "" || strings.Contains(filter, " ") || strings.Contains(filter, ":") {
			continue
		}
		if seen[filter] {
			continue
		}
		seen[filter] = true
		filters = append(filters, filter)
	}
	return filters
}

func indexOfString(values []string, needle string) int {
	for index, value := range values {
		if value == needle {
			return index
		}
	}
	return -1
}

func bindSwitch(widget *gtk.Switch, state *appState, update func(*settings, bool)) {
	widget.ConnectStateSet(func(value bool) bool {
		update(state.settings, value)
		saveSettings(*state.settings)
		return false
	})
}

func bindEntry(entry *gtk.Entry, state *appState, update func(*settings, string)) {
	save := func() {
		update(state.settings, entry.Text())
		saveSettings(*state.settings)
	}

	entry.ConnectActivate(save)
	entry.NotifyProperty("has-focus", func() {
		if !entry.HasFocus() {
			save()
		}
	})
}
