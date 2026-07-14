package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	ndl "github.com/mechakotik/ndl/lib"
)

type settings struct {
	PreferDiscreteGPU  bool
	PauseHidden        bool
	PauseOnBat         bool
	VSync              bool
	FPSLimit           int
	AudioVisualization bool
	AudioBackend       uint
	AudioSource        string
	VideoScaleMode     string
	VideoFilter        string
	SteamLibraryPath   string
	AutorunWallpapers  map[string]string
	WallpaperOptions   map[string]wallpaperOptions
}

type wallpaperOptions struct {
	Speed                        float64
	VSync                        bool
	VSyncOverridden              bool
	FPSLimit                     int
	FPSLimitOverridden           bool
	PreferDiscreteGPU            bool
	PreferDiscreteGPUOverridden  bool
	AudioVisualization           bool
	AudioVisualizationOverridden bool
	AudioBackend                 uint
	AudioBackendOverridden       bool
	AudioSource                  string
	AudioSourceOverridden        bool
	ScaleMode                    string
	ScaleModeOverridden          bool
	Filter                       string
	FilterOverridden             bool
	HiddenObjectIDs              []int
	HiddenEffects                map[int][]string
}

type settingsFile struct {
	PreferDiscreteGPU  bool                            `ndl:"prefer_discrete_gpu"`
	PauseHidden        bool                            `ndl:"pause_hidden"`
	PauseOnBat         bool                            `ndl:"pause_on_bat"`
	VSync              bool                            `ndl:"vsync"`
	FPSLimit           int                             `ndl:"fps_limit"`
	AudioVisualization bool                            `ndl:"audio_visualization"`
	AudioBackend       uint                            `ndl:"audio_backend"`
	AudioSource        string                          `ndl:"audio_source"`
	VideoScaleMode     string                          `ndl:"video_scale_mode"`
	VideoFilter        string                          `ndl:"video_filter"`
	SteamLibraryPath   string                          `ndl:"steam_library_path"`
	AutorunWallpapers  map[string]string               `ndl:"autorun_wallpapers"`
	WallpaperOptions   map[string]wallpaperOptionsData `ndl:"wallpaper_options,omitempty"`
}

type wallpaperOptionsData struct {
	Speed              *float64            `ndl:"speed,omitempty"`
	VSync              *bool               `ndl:"vsync,omitempty"`
	FPSLimit           *int                `ndl:"fps_limit,omitempty"`
	PreferDiscreteGPU  *bool               `ndl:"prefer_discrete_gpu,omitempty"`
	AudioVisualization *bool               `ndl:"audio_visualization,omitempty"`
	AudioBackend       *uint               `ndl:"audio_backend,omitempty"`
	AudioSource        *string             `ndl:"audio_source,omitempty"`
	ScaleMode          *string             `ndl:"scale_mode,omitempty"`
	Filter             *string             `ndl:"filter,omitempty"`
	HiddenObjects      []int               `ndl:"hidden_objects,omitempty"`
	HiddenEffects      []hiddenEffectsData `ndl:"hidden_effects,omitempty"`
}

type hiddenEffectsData struct {
	ObjectID int      `ndl:"object_id"`
	Effects  []string `ndl:"effects"`
}

type sceneWallpaperOptions struct {
	VSync              bool   `ndl:"vsync"`
	FPSLimit           int    `ndl:"fps_limit"`
	PreferDiscreteGPU  bool   `ndl:"prefer_discrete_gpu"`
	AudioVisualization bool   `ndl:"audio_visualization"`
	AudioBackend       uint   `ndl:"audio_backend"`
	AudioSource        string `ndl:"audio_source"`
}

type videoWallpaperOptions struct {
	ScaleMode string `ndl:"scale_mode"`
	Filter    string `ndl:"filter"`
}

type wallpaperKind int

const (
	wallpaperEngineScene wallpaperKind = iota + 1
	wallpaperEngineVideo
)

type wallpaper struct {
	title       string
	description string
	path        string
	launchPath  string
	previewPath string
	kind        wallpaperKind
	importDir   string
	assetsDir   string
}

type wallpaperMetadata struct {
	Title       string `ndl:"title"`
	Description string `ndl:"description"`
}

type detailWidgets struct {
	title                        *gtk.Label
	description                  *gtk.Label
	descriptionExpander          *gtk.Expander
	preview                      *previewWidget
	optionsBox                   *gtk.Box
	otherBox                     *gtk.Box
	speedSpin                    *gtk.SpinButton
	selectedRunButton            *gtk.Button
	selectedRunMenu              *gtk.MenuButton
	selectedAdvancedImportButton *gtk.Button
	bottomAdvancedImportButton   *gtk.Button
	deleteWallpaperButton        *gtk.Button
}

type previewWidget struct {
	overlay    *gtk.Overlay
	measure    *gtk.Box
	picture    *gtk.Picture
	loadCancel chan struct{}
}

type appState struct {
	settings               *settings
	wallpapers             []wallpaper
	selectedIndex          int
	selectedDisplay        string
	displayMappingsChanged []func()
	processMu              sync.Mutex
	processes              map[int]*wallpaperProcess
	dialogParent           gtk.Widgetter
	exiting                atomic.Bool
	updatingDetail         bool
	working                atomic.Bool
	refreshWallpapers      func()
}

func (state *appState) notifyDisplayMappingsChanged() {
	for _, callback := range state.displayMappingsChanged {
		callback()
	}
}

func (state *appState) onDisplayMappingsChanged(callback func()) {
	state.displayMappingsChanged = append(state.displayMappingsChanged, callback)
	callback()
}

func (currentSettings *settings) setWallpaperOptions(path string, options wallpaperOptions) {
	if currentSettings.WallpaperOptions == nil {
		currentSettings.WallpaperOptions = map[string]wallpaperOptions{}
	}
	currentSettings.WallpaperOptions[path] = normalizeWallpaperOptions(options)
}

func loadSettings() settings {
	currentSettings := defaultSettings()
	raw, err := os.ReadFile(settingsPath())
	if err != nil {
		return currentSettings
	}
	file := settingsFileFromSettings(currentSettings)
	if err := ndl.Unmarshal(string(raw), &file); err != nil {
		return defaultSettings()
	}
	currentSettings = settingsFromFile(file)
	return currentSettings
}

func settingsFromFile(data settingsFile) settings {
	currentSettings := settings{
		PreferDiscreteGPU:  data.PreferDiscreteGPU,
		PauseHidden:        data.PauseHidden,
		PauseOnBat:         data.PauseOnBat,
		VSync:              data.VSync,
		FPSLimit:           data.FPSLimit,
		AudioVisualization: data.AudioVisualization,
		AudioBackend:       data.AudioBackend,
		AudioSource:        data.AudioSource,
		VideoScaleMode:     data.VideoScaleMode,
		VideoFilter:        data.VideoFilter,
		SteamLibraryPath:   data.SteamLibraryPath,
		AutorunWallpapers:  data.AutorunWallpapers,
		WallpaperOptions:   wallpaperOptionsFromDataMap(data.WallpaperOptions),
	}
	if currentSettings.AutorunWallpapers == nil {
		currentSettings.AutorunWallpapers = map[string]string{}
	}
	if currentSettings.WallpaperOptions == nil {
		currentSettings.WallpaperOptions = map[string]wallpaperOptions{}
	}
	currentSettings.FPSLimit = normalizedFPSLimit(currentSettings.FPSLimit)
	currentSettings.AudioBackend = min(currentSettings.AudioBackend, 3)
	currentSettings.VideoScaleMode = normalizedScaleMode(currentSettings.VideoScaleMode)
	for path, options := range currentSettings.WallpaperOptions {
		currentSettings.WallpaperOptions[path] = normalizeWallpaperOptions(options)
	}
	return currentSettings
}

func saveSettings(currentSettings settings) {
	path := settingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create settings directory: %v\n", err)
		return
	}

	data, err := ndl.Marshal(settingsFileFromSettings(currentSettings))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to serialize settings: %v\n", err)
		return
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write settings: %v\n", err)
	}
}

func settingsFileFromSettings(currentSettings settings) settingsFile {
	return settingsFile{
		PreferDiscreteGPU:  currentSettings.PreferDiscreteGPU,
		PauseHidden:        currentSettings.PauseHidden,
		PauseOnBat:         currentSettings.PauseOnBat,
		VSync:              currentSettings.VSync,
		FPSLimit:           currentSettings.FPSLimit,
		AudioVisualization: currentSettings.AudioVisualization,
		AudioBackend:       currentSettings.AudioBackend,
		AudioSource:        currentSettings.AudioSource,
		VideoScaleMode:     currentSettings.VideoScaleMode,
		VideoFilter:        currentSettings.VideoFilter,
		SteamLibraryPath:   currentSettings.SteamLibraryPath,
		AutorunWallpapers:  currentSettings.AutorunWallpapers,
		WallpaperOptions:   wallpaperOptionsDataMap(currentSettings.WallpaperOptions),
	}
}

func wallpaperOptionsDataMap(source map[string]wallpaperOptions) map[string]wallpaperOptionsData {
	result := map[string]wallpaperOptionsData{}
	for path, options := range source {
		data := wallpaperOptionsDataFromOptions(options)
		if wallpaperOptionsDataIsEmpty(data) {
			continue
		}
		result[path] = data
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func wallpaperOptionsDataFromOptions(options wallpaperOptions) wallpaperOptionsData {
	options = normalizeWallpaperOptions(options)
	data := wallpaperOptionsData{}
	if options.Speed != defaultWallpaperOptions().Speed {
		data.Speed = valuePtr(options.Speed)
	}
	if options.VSyncOverridden {
		data.VSync = valuePtr(options.VSync)
	}
	if options.FPSLimitOverridden {
		data.FPSLimit = valuePtr(options.FPSLimit)
	}
	if options.PreferDiscreteGPUOverridden {
		data.PreferDiscreteGPU = valuePtr(options.PreferDiscreteGPU)
	}
	if options.AudioVisualizationOverridden {
		data.AudioVisualization = valuePtr(options.AudioVisualization)
	}
	if options.AudioBackendOverridden {
		data.AudioBackend = valuePtr(options.AudioBackend)
	}
	if options.AudioSourceOverridden {
		data.AudioSource = valuePtr(options.AudioSource)
	}
	if options.ScaleModeOverridden {
		data.ScaleMode = valuePtr(options.ScaleMode)
	}
	if options.FilterOverridden {
		data.Filter = valuePtr(options.Filter)
	}
	if len(options.HiddenObjectIDs) > 0 {
		data.HiddenObjects = options.HiddenObjectIDs
	}
	if len(options.HiddenEffects) > 0 {
		data.HiddenEffects = hiddenEffectsDataFromMap(options.HiddenEffects)
	}
	return data
}

func wallpaperOptionsDataIsEmpty(data wallpaperOptionsData) bool {
	return data.Speed == nil &&
		data.VSync == nil &&
		data.FPSLimit == nil &&
		data.PreferDiscreteGPU == nil &&
		data.AudioVisualization == nil &&
		data.AudioBackend == nil &&
		data.AudioSource == nil &&
		data.ScaleMode == nil &&
		data.Filter == nil &&
		len(data.HiddenObjects) == 0 &&
		len(data.HiddenEffects) == 0
}

func wallpaperOptionsFromDataMap(source map[string]wallpaperOptionsData) map[string]wallpaperOptions {
	result := map[string]wallpaperOptions{}
	for path, data := range source {
		result[path] = wallpaperOptionsFromData(data)
	}
	return result
}

func wallpaperOptionsFromData(data wallpaperOptionsData) wallpaperOptions {
	options := defaultWallpaperOptions()
	if data.Speed != nil {
		options.Speed = *data.Speed
	}
	if data.VSync != nil {
		options.VSync = *data.VSync
		options.VSyncOverridden = true
	}
	if data.FPSLimit != nil {
		options.FPSLimit = *data.FPSLimit
		options.FPSLimitOverridden = true
	}
	if data.PreferDiscreteGPU != nil {
		options.PreferDiscreteGPU = *data.PreferDiscreteGPU
		options.PreferDiscreteGPUOverridden = true
	}
	if data.AudioVisualization != nil {
		options.AudioVisualization = *data.AudioVisualization
		options.AudioVisualizationOverridden = true
	}
	if data.AudioBackend != nil {
		options.AudioBackend = *data.AudioBackend
		options.AudioBackendOverridden = true
	}
	if data.AudioSource != nil {
		options.AudioSource = *data.AudioSource
		options.AudioSourceOverridden = true
	}
	if data.ScaleMode != nil {
		options.ScaleMode = *data.ScaleMode
		options.ScaleModeOverridden = true
	}
	if data.Filter != nil {
		options.Filter = *data.Filter
		options.FilterOverridden = true
	}
	options.HiddenObjectIDs = data.HiddenObjects
	options.HiddenEffects = hiddenEffectsMapFromData(data.HiddenEffects)
	return normalizeWallpaperOptions(options)
}

func valuePtr[T any](value T) *T {
	return &value
}

func sceneOptionsForPath(currentSettings settings, path string) sceneWallpaperOptions {
	scene := globalSceneOptions(currentSettings)
	options := wallpaperOptionsForPath(currentSettings, path)
	return sceneOptionsWithOverrides(scene, options)
}

func videoOptionsForPath(currentSettings settings, path string) videoWallpaperOptions {
	video := globalVideoOptions(currentSettings)
	options := wallpaperOptionsForPath(currentSettings, path)
	return videoOptionsWithOverrides(video, options)
}

func sceneOptionsWithOverrides(scene sceneWallpaperOptions, options wallpaperOptions) sceneWallpaperOptions {
	if options.VSyncOverridden {
		scene.VSync = options.VSync
	}
	if options.FPSLimitOverridden {
		scene.FPSLimit = normalizedFPSLimit(options.FPSLimit)
	}
	if options.PreferDiscreteGPUOverridden {
		scene.PreferDiscreteGPU = options.PreferDiscreteGPU
	}
	if options.AudioVisualizationOverridden {
		scene.AudioVisualization = options.AudioVisualization
	}
	if options.AudioBackendOverridden {
		scene.AudioBackend = min(options.AudioBackend, 3)
	}
	if options.AudioSourceOverridden {
		scene.AudioSource = options.AudioSource
	}
	return scene
}

func videoOptionsWithOverrides(video videoWallpaperOptions, options wallpaperOptions) videoWallpaperOptions {
	if options.ScaleModeOverridden {
		video.ScaleMode = normalizedScaleMode(options.ScaleMode)
	}
	if options.FilterOverridden {
		video.Filter = options.Filter
	}
	return video
}

func wallpaperOptionsForPath(currentSettings settings, path string) wallpaperOptions {
	if currentSettings.WallpaperOptions == nil {
		return defaultWallpaperOptions()
	}
	return normalizeWallpaperOptions(currentSettings.WallpaperOptions[path])
}

func defaultSettings() settings {
	return settings{
		PauseHidden:        true,
		FPSLimit:           30,
		AudioVisualization: true,
		VideoScaleMode:     "aspect-crop",
		AutorunWallpapers:  map[string]string{},
		WallpaperOptions:   map[string]wallpaperOptions{},
	}
}

func defaultWallpaperOptions() wallpaperOptions {
	return wallpaperOptions{
		Speed: 1.0,
	}
}

func normalizeWallpaperOptions(options wallpaperOptions) wallpaperOptions {
	if options.Speed <= 0 {
		options.Speed = 1.0
	}
	if options.FPSLimitOverridden {
		options.FPSLimit = normalizedFPSLimit(options.FPSLimit)
	}
	if options.AudioBackendOverridden {
		options.AudioBackend = min(options.AudioBackend, 3)
	}
	if options.ScaleModeOverridden {
		options.ScaleMode = normalizedScaleMode(options.ScaleMode)
	}
	options.HiddenObjectIDs = normalizeIntList(options.HiddenObjectIDs)
	options.HiddenEffects = normalizeHiddenEffects(options.HiddenEffects)
	return options
}

func hiddenEffectsDataFromMap(source map[int][]string) []hiddenEffectsData {
	if len(source) == 0 {
		return nil
	}

	objectIDs := make([]int, 0, len(source))
	for objectID := range source {
		objectIDs = append(objectIDs, objectID)
	}
	slices.Sort(objectIDs)

	result := []hiddenEffectsData{}
	for _, objectID := range objectIDs {
		effects := normalizeStringList(source[objectID])
		if len(effects) == 0 {
			continue
		}
		result = append(result, hiddenEffectsData{
			ObjectID: objectID,
			Effects:  effects,
		})
	}
	return result
}

func hiddenEffectsMapFromData(source []hiddenEffectsData) map[int][]string {
	result := map[int][]string{}
	for _, entry := range source {
		effects := normalizeStringList(entry.Effects)
		if len(effects) == 0 {
			continue
		}
		result[entry.ObjectID] = effects
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeHiddenEffects(source map[int][]string) map[int][]string {
	result := map[int][]string{}
	for objectID, effects := range source {
		effects = normalizeStringList(effects)
		if len(effects) == 0 {
			continue
		}
		result[objectID] = effects
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeIntList(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	values = append([]int(nil), values...)
	slices.Sort(values)
	values = slices.Compact(values)
	return values
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	values = append([]string(nil), values...)
	slices.Sort(values)
	values = slices.Compact(values)
	values = slices.DeleteFunc(values, func(value string) bool {
		return value == ""
	})
	if len(values) == 0 {
		return nil
	}
	return values
}

func globalSceneOptions(currentSettings settings) sceneWallpaperOptions {
	return sceneWallpaperOptions{
		VSync:              currentSettings.VSync,
		FPSLimit:           normalizedFPSLimit(currentSettings.FPSLimit),
		PreferDiscreteGPU:  currentSettings.PreferDiscreteGPU,
		AudioVisualization: currentSettings.AudioVisualization,
		AudioBackend:       min(currentSettings.AudioBackend, 3),
		AudioSource:        currentSettings.AudioSource,
	}
}

func globalVideoOptions(currentSettings settings) videoWallpaperOptions {
	return videoWallpaperOptions{
		ScaleMode: normalizedScaleMode(currentSettings.VideoScaleMode),
		Filter:    currentSettings.VideoFilter,
	}
}

func normalizedFPSLimit(value int) int {
	if value <= 0 {
		return 30
	}
	return value
}

func settingsPath() string {
	return filepath.Join(configHome(), "owui", "settings.ndl")
}

func configHome() string {
	if path := os.Getenv("XDG_CONFIG_HOME"); path != "" {
		return path
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config")
	}
	return "."
}
