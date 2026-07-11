package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/alexflint/go-arg"
)

var (
	env struct {
		Assets string
	}
	args struct {
		Input          string `arg:"positional,required"`
		Output         string `arg:"positional"`
		Particles      bool   `arg:"--particles" default:"true"`
		KeepSources    bool   `arg:"--keep-sources"`
		ListObjects    bool   `arg:"--list-objects"`
		ListObjectsNDL bool   `arg:"--list-objects-ndl"`
		SkipObjects    string `arg:"--skip-objects"`
		SkipEffects    string `arg:"--skip-effects"`
	}
	state struct {
		PKGMap    map[string][]byte
		Scene     Scene
		Tasks     []any
		OutputMap map[string][]byte
		Mutex     sync.Mutex
	}
)

type inputDir struct {
	Root        string
	PackagePath string
	ProjectPath string
}

func main() {
	arg.MustParse(&args)

	input, err := resolveInputDir(args.Input)
	if err != nil {
		panic("invalid input: " + err.Error())
	}

	project, err := loadProject(input.ProjectPath)
	if err != nil {
		panic("parse project.json failed: " + err.Error())
	}

	state.OutputMap = map[string][]byte{}
	if !listObjectsRequested() {
		makeMetadata(input.ProjectPath, project, state.OutputMap)
	}

	switch strings.ToLower(project.Type) {
	case "video":
		if err := writeVideoWallpaper(input, project); err != nil {
			panic("write video wallpaper failed: " + err.Error())
		}
	case "scene":
		if err := writeSceneWallpaper(input, project); err != nil {
			panic(err.Error())
		}
	default:
		if project.Type == "" && project.Category != "" {
			panic(fmt.Sprintf("unsupported project category %q", project.Category))
		}
		panic(fmt.Sprintf("unsupported project type %q", project.Type))
	}
}

func listObjectsRequested() bool {
	return args.ListObjects || args.ListObjectsNDL
}

func resolveInputDir(root string) (inputDir, error) {
	info, err := os.Stat(root)
	if err != nil {
		return inputDir{}, err
	}
	if !info.IsDir() {
		return inputDir{}, fmt.Errorf("%s is not a directory", root)
	}

	projectPath := filepath.Join(root, "project.json")

	return inputDir{
		Root:        root,
		PackagePath: filepath.Join(root, "scene.pkg"),
		ProjectPath: projectPath,
	}, nil
}

func requireRegularFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s not found", path)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}
	return nil
}

func inputAssetPath(root string, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty input file name")
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("input file %q is absolute", name)
	}

	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("input file %q escapes input directory", name)
	}

	path := filepath.Join(root, clean)
	if err := requireRegularFile(path); err != nil {
		return "", err
	}
	return path, nil
}

func writeVideoWallpaper(input inputDir, project Project) error {
	if listObjectsRequested() {
		return fmt.Errorf("cannot list objects for video wallpaper")
	}
	if args.Output == "" {
		return fmt.Errorf("output path is required")
	}
	if strings.ToLower(filepath.Ext(project.File)) != ".mp4" {
		return fmt.Errorf("video wallpaper file %q is not mp4", project.File)
	}

	videoPath, err := inputAssetPath(input.Root, project.File)
	if err != nil {
		return err
	}
	if err := writeOutputDir(args.Output, state.OutputMap); err != nil {
		return err
	}
	return copyOutputFile(args.Output, "video.mp4", videoPath)
}

func outputPath(root string, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty output file name")
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("output file %q is absolute", name)
	}

	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("output file %q escapes output directory", name)
	}

	return filepath.Join(root, clean), nil
}

func writeOutputDir(root string, files map[string][]byte) error {
	cleanRoot := filepath.Clean(root)
	if cleanRoot == "." || cleanRoot == string(filepath.Separator) {
		return fmt.Errorf("refusing to overwrite output directory %q", root)
	}

	if err := os.RemoveAll(root); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}

	for name, data := range files {
		path, err := outputPath(root, name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return err
		}
	}
	return nil
}

func copyOutputFile(root string, name string, src string) error {
	dst, err := outputPath(root, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
