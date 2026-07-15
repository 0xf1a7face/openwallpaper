package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	ndl "github.com/mechakotik/ndl/lib"
)

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

type wallpaperEngineProject struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	File        string `json:"file"`
	Preview     string `json:"preview"`
	Type        string `json:"type"`
}

func loadWallpapers(currentSettings settings) []wallpaper {
	if userDir := userOwuiDir(); userDir != "" {
		_ = os.MkdirAll(filepath.Join(userDir, "local"), 0o755)
	}

	wallpapers := loadWallpaperEngineWallpapers(currentSettings)
	wallpapers = append(wallpapers, loadNativeWallpapers(currentWallpaperEngineImportDirs(wallpapers))...)

	sort.Slice(wallpapers, func(left, right int) bool {
		return strings.ToLower(wallpapers[left].title) < strings.ToLower(wallpapers[right].title)
	})

	return wallpapers
}

func currentWallpaperEngineImportDirs(wallpapers []wallpaper) map[string]bool {
	dirs := map[string]bool{}
	for _, wallpaper := range wallpapers {
		if wallpaper.kind == wallpaperEngineScene && wallpaper.importDir != "" {
			dirs[filepath.Clean(wallpaper.importDir)] = true
		}
	}
	return dirs
}

func loadNativeWallpapers(skipDirs map[string]bool) []wallpaper {
	wallpapers := []wallpaper{}
	for _, root := range owuiDirs() {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || !entry.IsDir() {
				return nil
			}
			if path != root && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			if skipDirs[filepath.Clean(path)] {
				return filepath.SkipDir
			}

			launchPath := wallpaperLaunchTarget(path)
			if launchPath == "" {
				return nil
			}

			wallpapers = append(wallpapers, loadWallpaper(path, launchPath, filepath.Base(path)))
			return filepath.SkipDir
		})
		if err != nil {
			continue
		}
	}
	return wallpapers
}

func loadWallpaperEngineWallpapers(currentSettings settings) []wallpaper {
	steamLibrary := steamLibraryPath(currentSettings)
	workshopDir := filepath.Join(steamLibrary, "steamapps", "workshop", "content", "431960")
	assetsDir := wallpaperEngineAssetsDir(steamLibrary)
	entries, err := os.ReadDir(workshopDir)
	if err != nil {
		return nil
	}

	wallpapers := make([]wallpaper, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		wallpaperPath := filepath.Join(workshopDir, entry.Name())
		if wallpaper, ok := loadWallpaperEngineWallpaper(wallpaperPath, assetsDir); ok {
			wallpapers = append(wallpapers, wallpaper)
		}
	}
	return wallpapers
}

func loadWallpaperEngineWallpaper(path string, assetsDir string) (wallpaper, bool) {
	project, err := loadWallpaperEngineProject(filepath.Join(path, "project.json"))
	if err != nil {
		return wallpaper{}, false
	}

	workshopID := filepath.Base(path)
	title := project.Title
	if title == "" {
		title = workshopID
	}

	previewPath, _ := inputAssetPath(path, project.Preview)

	switch strings.ToLower(project.Type) {
	case "scene":
		return wallpaper{
			title:       title,
			description: project.Description,
			path:        path,
			previewPath: previewPath,
			kind:        wallpaperEngineScene,
			importDir:   wallpaperEngineImportDir(workshopID),
			assetsDir:   assetsDir,
		}, true
	case "video":
		videoPath, err := inputAssetPath(path, project.File)
		if err != nil || !isVideoFile(videoPath) {
			return wallpaper{}, false
		}
		return wallpaper{
			title:       title,
			description: project.Description,
			path:        path,
			launchPath:  videoPath,
			previewPath: previewPath,
			kind:        wallpaperEngineVideo,
		}, true
	default:
		return wallpaper{}, false
	}
}

func loadWallpaperEngineProject(path string) (wallpaperEngineProject, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return wallpaperEngineProject{}, err
	}

	project := wallpaperEngineProject{}
	err = json.Unmarshal(data, &project)
	return project, err
}

func steamLibraryPath(currentSettings settings) string {
	if currentSettings.SteamLibraryPath != "" {
		return expandHomePath(currentSettings.SteamLibraryPath)
	}
	return defaultSteamLibraryPath()
}

func wallpaperEngineAssetsDir(steamLibrary string) string {
	return filepath.Join(steamLibrary, "steamapps", "common", "wallpaper_engine", "assets")
}

func defaultSteamLibraryPath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "share", "Steam")
	}
	return filepath.Join(".", "Steam")
}

func expandHomePath(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func wallpaperEngineImportDir(workshopID string) string {
	if userDir := userOwuiDir(); userDir != "" {
		return filepath.Join(userDir, "wpe", workshopID)
	}
	return filepath.Join(".", "owui", "wpe", workshopID)
}

func inputAssetPath(root string, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty asset path")
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("asset path %q is absolute", name)
	}

	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("asset path %q escapes wallpaper directory", name)
	}

	path := filepath.Join(root, clean)
	if !regularFileExists(path) {
		return "", fmt.Errorf("asset path %q is not a regular file", name)
	}
	return path, nil
}

func owuiDirs() []string {
	dirs := []string{}
	if userDir := userOwuiDir(); userDir != "" {
		dirs = append(dirs, userDir)
	}
	return append(dirs, "/usr/share/owui", "/usr/local/share/owui")
}

func userOwuiDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "share", "owui")
	}
	return ""
}

func wallpaperLaunchTarget(path string) string {
	scenePath := filepath.Join(path, "scene.wasm")
	if regularFileExists(scenePath) {
		return scenePath
	}

	videoPath := filepath.Join(path, "video.mp4")
	if regularFileExists(videoPath) {
		return videoPath
	}
	if project, err := loadWallpaperEngineProject(filepath.Join(path, "project.json")); err == nil && strings.EqualFold(project.Type, "video") {
		if videoPath, err := inputAssetPath(path, project.File); err == nil && isVideoFile(videoPath) {
			return videoPath
		}
	}
	return ""
}

func isSceneFile(path string) bool {
	return filepath.Base(path) == "scene.wasm"
}

func isVideoFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".mp4")
}

func (wallpaper wallpaper) runnableLaunchPath() string {
	if wallpaper.kind == wallpaperEngineScene {
		return wallpaper.wallpaperEngineImportedLaunchPath()
	}
	return wallpaper.launchPath
}

func (wallpaper wallpaper) optionsPath() string {
	if wallpaper.kind == wallpaperEngineScene && wallpaper.importDir != "" {
		return wallpaper.importDir
	}
	if wallpaper.path != "" {
		return wallpaper.path
	}
	if wallpaper.launchPath != "" {
		return filepath.Dir(wallpaper.launchPath)
	}
	return ""
}

func (wallpaper wallpaper) wallpaperEngineImportedLaunchPath() string {
	if wallpaper.importDir == "" {
		return ""
	}
	return wallpaperLaunchTarget(wallpaper.importDir)
}

func (wallpaper wallpaper) importedWallpaperEngineScene() bool {
	return wallpaper.kind == wallpaperEngineScene && wallpaper.runnableLaunchPath() != ""
}

func (wallpaper wallpaper) canDelete() bool {
	_, ok := wallpaper.deletePath()
	return ok
}

func (wallpaper wallpaper) deletePath() (string, bool) {
	switch wallpaper.kind {
	case wallpaperEngineScene:
		if wallpaper.importedWallpaperEngineScene() {
			return wallpaper.importDir, true
		}
		return "", false
	case wallpaperEngineVideo:
		return "", false
	default:
		return wallpaper.path, wallpaper.path != ""
	}
}

func deleteWallpaperFiles(wallpaper wallpaper) (string, bool, error) {
	path, ok := wallpaper.deletePath()
	if !ok {
		return "", false, nil
	}

	clean := filepath.Clean(path)
	if clean == "." || clean == string(filepath.Separator) {
		return path, true, fmt.Errorf("refusing to delete unsafe wallpaper path %q", path)
	}
	return path, true, os.RemoveAll(path)
}

func loadWallpaper(path string, launchPath string, fallbackTitle string) wallpaper {
	title, description := loadWallpaperMetadata(path, fallbackTitle)
	return wallpaper{
		title:       title,
		description: description,
		path:        path,
		launchPath:  launchPath,
		previewPath: previewPath(path),
	}
}

func loadWallpaperMetadata(path string, fallbackTitle string) (string, string) {
	title := fallbackTitle
	description := ""

	data, err := os.ReadFile(filepath.Join(path, "metadata.ndl"))
	if err != nil {
		return title, description
	}

	metadata := wallpaperMetadata{}
	if err := ndl.Unmarshal(string(data), &metadata); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse wallpaper metadata %s: %v\n", filepath.Join(path, "metadata.ndl"), err)
		return title, description
	}

	if metadata.Title != "" {
		title = metadata.Title
	}
	description = metadata.Description
	return title, description
}

func previewPath(path string) string {
	preview := filepath.Join(path, "preview.webp")
	if regularFileExists(preview) {
		return preview
	}
	return ""
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
