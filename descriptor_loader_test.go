package grpc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func TestBuildAndRegisterDescriptors(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T) *Plugin
		expectedError bool
		verify        func(t *testing.T, p *Plugin)
	}{
		{
			name: "no proto files configured",
			setup: func(_ *testing.T) *Plugin {
				return &Plugin{
					config: &Config{
						Proto: []string{},
						Reflection: &ReflectionConfig{
							ImportPaths: []string{},
						},
					},
					log: zap.NewNop(),
				}
			},
			expectedError: false,
			verify: func(t *testing.T, _ *Plugin) {
				var fileCount int
				protoregistry.GlobalFiles.RangeFiles(func(_ protoreflect.FileDescriptor) bool {
					fileCount++
					return true
				})
				assert.Equal(t, 0, fileCount, "no files should be registered")
			},
		},
		{
			name: "compile real proto files from tests directory",
			setup: func(t *testing.T) *Plugin {
				protoPath := filepath.Join("tests", "proto", "test", "test.proto")
				require.FileExists(t, protoPath, "test.proto should exist in tests/proto/test/")

				return &Plugin{
					config: &Config{
						Proto: []string{protoPath},
						Reflection: &ReflectionConfig{
							ImportPaths: []string{filepath.Join("tests", "proto", "test")},
						},
					},
					log: zap.NewNop(),
				}
			},
			expectedError: false,
			verify: func(t *testing.T, _ *Plugin) {
				var registeredFiles []string
				protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
					registeredFiles = append(registeredFiles, fd.Path())
					return true
				})

				if len(registeredFiles) == 0 {
					t.Fatal("No files were registered!")
				}

				fileDesc, err := protoregistry.GlobalFiles.FindFileByPath("test.proto")
				if err != nil {
					for _, path := range []string{"tests/proto/test/test.proto", "test/test.proto"} {
						if fd, err2 := protoregistry.GlobalFiles.FindFileByPath(path); err2 == nil {
							fileDesc = fd
							err = nil
							break
						}
					}
				}
				require.NoError(t, err, "test.proto file should be registered")

				assert.Equal(t, "test", string(fileDesc.Package()))

				services := fileDesc.Services()
				require.Greater(t, services.Len(), 0, "should have at least one service")

				serviceDesc := services.Get(0)
				assert.Equal(t, "Test", string(serviceDesc.Name()))

				methods := serviceDesc.Methods()
				assert.Greater(t, methods.Len(), 0, "Test service should have methods")
			},
		},
		{
			name: "duplicate proto files",
			setup: func(t *testing.T) *Plugin {
				protoPath := filepath.Join("tests", "proto", "test", "test.proto")
				require.FileExists(t, protoPath)

				return &Plugin{
					config: &Config{
						Proto: []string{protoPath, protoPath}, // Same file twice
						Reflection: &ReflectionConfig{
							ImportPaths: []string{filepath.Join("tests", "proto", "test")},
						},
					},
					log: zap.NewNop(),
				}
			},
			expectedError: false,
			verify: func(t *testing.T, _ *Plugin) {
				var allFiles []string
				var testProtoCount int
				protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
					allFiles = append(allFiles, fd.Path())
					if fd.Path() == "test.proto" {
						testProtoCount++
					}
					return true
				})

				assert.Greater(t, len(allFiles), 0, "at least one file should be registered")

				assert.Equal(t, 1, testProtoCount, "test.proto should be registered only once")
			},
		},
		{
			name: "invalid proto file path",
			setup: func(_ *testing.T) *Plugin {
				return &Plugin{
					config: &Config{
						Proto: []string{"nonexistent/invalid.proto"},
						Reflection: &ReflectionConfig{
							ImportPaths: []string{"."},
						},
					},
					log: zap.NewNop(),
				}
			},
			expectedError: true,
			verify:        nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protoregistry.GlobalFiles = new(protoregistry.Files)

			p := tt.setup(t)
			err := p.buildAndRegisterDescriptors()

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.verify != nil {
				tt.verify(t, p)
			}
		})
	}
}

func TestBuildImportPaths(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(t *testing.T) *Plugin
		verify func(t *testing.T, paths []string)
	}{
		{
			name: "explicit import paths",
			setup: func(_ *testing.T) *Plugin {
				return &Plugin{
					config: &Config{
						Proto: []string{filepath.Join("tests", "proto", "test", "test.proto")},
						Reflection: &ReflectionConfig{
							ImportPaths: []string{filepath.Join("tests", "proto", "test")},
						},
					},
					log: zap.NewNop(),
				}
			},
			verify: func(t *testing.T, paths []string) {
				assert.Greater(t, len(paths), 0)

				hasTestDir := false
				for _, p := range paths {
					if strings.Contains(p, filepath.Join("tests", "proto", "test")) {
						hasTestDir = true
						break
					}
				}
				assert.True(t, hasTestDir, "should contain tests/proto/test directory")
			},
		},
		{
			name: "includes current working directory",
			setup: func(_ *testing.T) *Plugin {
				return &Plugin{
					config: &Config{
						Proto: []string{filepath.Join("tests", "proto", "test", "test.proto")},
						Reflection: &ReflectionConfig{
							ImportPaths: []string{},
						},
					},
					log: zap.NewNop(),
				}
			},
			verify: func(t *testing.T, paths []string) {
				assert.Greater(t, len(paths), 0)

				cwd, err := os.Getwd()
				require.NoError(t, err)

				assert.Contains(t, paths, cwd, "should include current working directory")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.setup(t)
			paths := p.buildImportPaths()
			tt.verify(t, paths)
		})
	}
}

func TestConvertToRelativePaths(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) (*Plugin, []string)
		expectedErr bool
		verify      func(t *testing.T, relativePaths []string)
	}{
		{
			name: "convert absolute to relative",
			setup: func(t *testing.T) (*Plugin, []string) {
				protoPath := filepath.Join("tests", "proto", "test", "test.proto")
				absPath, err := filepath.Abs(protoPath)
				require.NoError(t, err)

				importPaths := []string{filepath.Dir(absPath)}

				p := &Plugin{
					config: &Config{
						Proto: []string{absPath},
					},
					log: zap.NewNop(),
				}

				return p, importPaths
			},
			expectedErr: false,
			verify: func(t *testing.T, relativePaths []string) {
				require.Len(t, relativePaths, 1)
				assert.Equal(t, "test.proto", relativePaths[0])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, importPaths := tt.setup(t)
			relativePaths, err := p.convertToRelativePaths(importPaths)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.verify != nil {
					tt.verify(t, relativePaths)
				}
			}
		})
	}
}

func TestBuildFileMap(t *testing.T) {
	tests := []struct {
		name        string
		importPaths []string
		verify      func(t *testing.T, fileMap map[string]string)
	}{
		{
			name:        "scan tests proto directory",
			importPaths: []string{filepath.Join("tests", "proto", "test")},
			verify: func(t *testing.T, fileMap map[string]string) {
				assert.Greater(t, len(fileMap), 0, "should find proto files")

				// Check that proto files are in the map
				hasProtoFiles := false
				for key := range fileMap {
					if filepath.Ext(key) == ".proto" || strings.HasSuffix(key, ".proto") {
						hasProtoFiles = true
						break
					}
				}
				assert.True(t, hasProtoFiles, "should contain .proto files")
			},
		},
		{
			name:        "empty import paths",
			importPaths: []string{},
			verify: func(t *testing.T, fileMap map[string]string) {
				assert.Equal(t, 0, len(fileMap), "should be empty for no import paths")
			},
		},
		{
			name:        "non-existent directory",
			importPaths: []string{"nonexistent_directory_12345"},
			verify: func(t *testing.T, fileMap map[string]string) {
				assert.Equal(t, 0, len(fileMap), "should be empty for non-existent directory")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plugin{
				config: &Config{},
				log:    zap.NewNop(),
			}

			fileMap := p.buildFileMap(tt.importPaths)
			tt.verify(t, fileMap)
		})
	}
}

func TestCreateSmartAccessor(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) (map[string]string, []string)
		filename    string
		shouldFind  bool
		description string
	}{
		{
			name: "find by basename",
			setup: func(_ *testing.T) (map[string]string, []string) {
				fileMap := map[string]string{
					"test.proto": "syntax = \"proto3\";",
				}
				return fileMap, []string{"parser"}
			},
			filename:    "test.proto",
			shouldFind:  true,
			description: "should find file by basename",
		},
		{
			name: "skip google protobuf imports",
			setup: func(_ *testing.T) (map[string]string, []string) {
				fileMap := map[string]string{}
				return fileMap, []string{}
			},
			filename:    "google/protobuf/empty.proto",
			shouldFind:  false,
			description: "should skip google/protobuf imports",
		},
		{
			name: "skip google api imports",
			setup: func(_ *testing.T) (map[string]string, []string) {
				fileMap := map[string]string{}
				return fileMap, []string{}
			},
			filename:    "google/api/annotations.proto",
			shouldFind:  false,
			description: "should skip google/api imports",
		},
		{
			name: "find by relative path",
			setup: func(_ *testing.T) (map[string]string, []string) {
				fileMap := map[string]string{
					"parser/test.proto": "syntax = \"proto3\";",
				}
				return fileMap, []string{"parser"}
			},
			filename:    "parser/test.proto",
			shouldFind:  true,
			description: "should find file by relative path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileMap, importPaths := tt.setup(t)
			p := &Plugin{
				log: zap.NewNop(),
			}

			accessor := p.createSmartAccessor(fileMap, importPaths)
			reader, err := accessor(tt.filename)

			if tt.shouldFind {
				assert.NoError(t, err, tt.description)
				assert.NotNil(t, reader)
				if reader != nil {
					reader.Close()
				}
			} else {
				assert.Error(t, err, tt.description)
			}
		})
	}
}
