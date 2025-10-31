package grpc

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
	"github.com/bufbuild/protocompile/reporter"
	"go.uber.org/zap"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func (p *Plugin) buildAndRegisterDescriptors() error {
	if len(p.config.Proto) == 0 {
		return nil
	}

	importPaths := p.buildImportPaths()
	relativeProtoFiles, err := p.convertToRelativePaths(importPaths)
	if err != nil {
		return fmt.Errorf("failed to convert proto paths: %w", err)
	}

	fileMap := p.buildFileMap(importPaths)

	errorReporter := reporter.NewReporter(
		func(err reporter.ErrorWithPos) error {
			p.log.Error("proto compilation error",
				zap.String("file", err.GetPosition().Filename),
				zap.Int("line", err.GetPosition().Line),
				zap.String("message", err.Unwrap().Error()))
			return nil
		},
		func(err reporter.ErrorWithPos) {
			p.log.Warn("proto compilation warning",
				zap.String("file", err.GetPosition().Filename),
				zap.Int("line", err.GetPosition().Line),
				zap.String("message", err.Unwrap().Error()))
		},
	)

	customAccessor := p.createSmartAccessor(fileMap, importPaths)

	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: importPaths,
			Accessor:    customAccessor,
		}),
		Reporter: errorReporter,
	}

	ctx := context.Background()
	compiled, err := compiler.Compile(ctx, relativeProtoFiles...)
	if err != nil {
		p.log.Error("failed to compile proto files", zap.Error(err))
		return fmt.Errorf("proto compilation failed: %w", err)
	}

	fileDescriptors := convertLinkerFilesToDescriptors(compiled)
	registeredCount, skippedCount := p.registerDescriptors(fileDescriptors)

	p.log.Info("proto descriptors registered",
		zap.Int("registered", registeredCount),
		zap.Int("skipped", skippedCount),
		zap.Int("total", len(fileDescriptors)))

	return nil
}

func convertLinkerFilesToDescriptors(files linker.Files) []protoreflect.FileDescriptor {
	descriptors := make([]protoreflect.FileDescriptor, 0, len(files))
	for _, file := range files {
		descriptors = append(descriptors, file)
	}
	return descriptors
}

func (p *Plugin) registerDescriptors(descriptors []protoreflect.FileDescriptor) (registered, skipped int) {
	for _, fileDesc := range descriptors {
		fileName := fileDesc.Path()

		if _, err := protoregistry.GlobalFiles.FindFileByPath(fileName); err == nil {
			skipped++
			continue
		}

		if err := protoregistry.GlobalFiles.RegisterFile(fileDesc); err != nil {
			p.log.Warn("failed to register file descriptor",
				zap.String("file", fileName),
				zap.Error(err))
			continue
		}

		registered++
	}

	return registered, skipped
}

func (p *Plugin) buildImportPaths() []string {
	importPathsMap := make(map[string]bool)

	if p.config.Reflection != nil && len(p.config.Reflection.ImportPaths) > 0 {
		for _, path := range p.config.Reflection.ImportPaths {
			if path == "" {
				continue
			}

			absPath, err := filepath.Abs(path)
			if err != nil {
				p.log.Warn("cannot resolve import path", zap.String("path", path), zap.Error(err))
				continue
			}

			importPathsMap[absPath] = true

			p.addAllSubdirectories(absPath, importPathsMap)
		}

		for _, protoFile := range p.config.Proto {
			absFile, err := filepath.Abs(protoFile)
			if err != nil {
				continue
			}
			dir := filepath.Dir(absFile)
			if !importPathsMap[dir] {
				importPathsMap[dir] = true
			}
		}
	} else {
		for _, protoFile := range p.config.Proto {
			absFile, err := filepath.Abs(protoFile)
			if err != nil {
				continue
			}
			dir := filepath.Dir(absFile)
			importPathsMap[dir] = true

			currentDir := dir
			for i := 0; i < 5; i++ {
				parent := filepath.Dir(currentDir)
				if parent == currentDir || parent == "." || parent == "/" {
					break
				}

				importPathsMap[parent] = true

				if strings.HasSuffix(parent, "proto") {
					p.addAllSubdirectories(parent, importPathsMap)
					break
				}
				currentDir = parent
			}
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		importPathsMap[cwd] = true
	}

	importPaths := make([]string, 0, len(importPathsMap))
	for path := range importPathsMap {
		importPaths = append(importPaths, path)
	}

	return importPaths
}

func (p *Plugin) addAllSubdirectories(rootPath string, pathsMap map[string]bool) {
	info, err := os.Stat(rootPath)
	if err != nil || !info.IsDir() {
		return
	}

	maxDepth := 10
	visited := make(map[string]bool)

	var walkDir func(string, int)
	walkDir = func(dir string, depth int) {
		if depth > maxDepth {
			return
		}

		absDir, err := filepath.EvalSymlinks(dir)
		if err != nil {
			absDir = dir
		}

		if visited[absDir] {
			return
		}
		visited[absDir] = true

		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			name := entry.Name()

			subdirPath := filepath.Join(dir, name)
			pathsMap[subdirPath] = true
			walkDir(subdirPath, depth+1)
		}
	}

	walkDir(rootPath, 0)
}

func (p *Plugin) convertToRelativePaths(importPaths []string) ([]string, error) {
	relativeProtoFiles := make([]string, 0, len(p.config.Proto))

	for _, protoFile := range p.config.Proto {
		absProtoFile, err := filepath.Abs(protoFile)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve absolute path for %s: %w", protoFile, err)
		}

		found := false
		for _, importPath := range importPaths {
			relPath, err := filepath.Rel(importPath, absProtoFile)
			if err == nil && !filepath.IsAbs(relPath) && !strings.HasPrefix(relPath, "..") {
				relativeProtoFiles = append(relativeProtoFiles, filepath.ToSlash(relPath))
				found = true
				break
			}
		}

		if !found {
			relativeProtoFiles = append(relativeProtoFiles, absProtoFile)
			p.log.Warn("using absolute path as fallback",
				zap.String("file", protoFile),
				zap.String("absolute", absProtoFile))
		}
	}

	return relativeProtoFiles, nil
}

func (p *Plugin) buildFileMap(importPaths []string) map[string]string {
	fileMap := make(map[string]string)

	for _, importPath := range importPaths {
		err := filepath.Walk(importPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			if info.IsDir() {
				if strings.HasPrefix(info.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}

			if !strings.HasSuffix(info.Name(), ".proto") {
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				p.log.Warn("cannot read proto file",
					zap.String("path", path),
					zap.Error(err))
				return nil
			}

			relPath, err := filepath.Rel(importPath, path)
			if err == nil {
				key := filepath.ToSlash(relPath)
				fileMap[key] = string(content)
			}

			filename := info.Name()
			if _, exists := fileMap[filename]; !exists {
				fileMap[filename] = string(content)
			}

			absPath, err := filepath.Abs(path)
			if err == nil {
				fileMap[absPath] = string(content)
			}

			return nil
		})

		if err != nil {
			p.log.Warn("error walking import path",
				zap.String("path", importPath),
				zap.Error(err))
		}
	}

	return fileMap
}

func (p *Plugin) createSmartAccessor(fileMap map[string]string, importPaths []string) func(string) (io.ReadCloser, error) {
	return func(filename string) (io.ReadCloser, error) {
		if strings.Contains(filename, "google/protobuf/") {
			return nil, os.ErrNotExist
		}

		if strings.Contains(filename, "google/api/") {
			return nil, os.ErrNotExist
		}

		if content, exists := fileMap[filename]; exists {
			return io.NopCloser(strings.NewReader(content)), nil
		}

		baseName := filepath.Base(filename)
		if content, exists := fileMap[baseName]; exists {
			return io.NopCloser(strings.NewReader(content)), nil
		}

		normalizedPath := filepath.ToSlash(filepath.Clean(filename))
		if content, exists := fileMap[normalizedPath]; exists {
			return io.NopCloser(strings.NewReader(content)), nil
		}

		for _, importPath := range importPaths {
			fullPath := filepath.Join(importPath, filename)
			if data, err := os.ReadFile(fullPath); err == nil {
				return io.NopCloser(strings.NewReader(string(data))), nil
			}
		}

		for _, importPath := range importPaths {
			relPath := strings.TrimPrefix(filename, importPath)
			relPath = strings.TrimPrefix(relPath, string(filepath.Separator))

			if content, exists := fileMap[relPath]; exists {
				return io.NopCloser(strings.NewReader(content)), nil
			}
		}

		for key, content := range fileMap {
			if strings.HasSuffix(key, filename) || strings.HasSuffix(filename, key) {
				return io.NopCloser(strings.NewReader(content)), nil
			}
		}

		p.log.Warn("import file not found",
			zap.String("filename", filename),
			zap.Int("paths_searched", len(importPaths)))

		return nil, os.ErrNotExist
	}
}
