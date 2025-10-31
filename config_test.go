package grpc

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

const separator = string(filepath.Separator)

func TestInitDefaults(t *testing.T) {
	c := Config{}
	assert.Error(t, c.InitDefaults())

	c.Listen = "localhost:1234"
	assert.NoError(t, c.InitDefaults())

	c.Proto = []string{""}
	assert.NoError(t, c.InitDefaults())
	assert.Empty(t, c.Proto)

	c.Proto = []string{"parser/pong.proto"}
	assert.NoError(t, c.InitDefaults())
	assert.Equal(t, []string{"parser/pong.proto"}, c.Proto)

	c.Proto = []string{"parser/nonexitent*.notproto"}
	assert.NoError(t, c.InitDefaults())
	assert.Empty(t, c.Proto)

	c.Proto = []string{"config.go", "parser/*.proto"}
	assert.NoError(t, c.InitDefaults())
	assert.Equal(t, []string{
		"config.go",
		"parser" + separator + "message.proto",
		"parser" + separator + "pong.proto",
		"parser" + separator + "test.proto",
		"parser" + separator + "test_import.proto",
	}, c.Proto)

	c.Proto = []string{"parser/?est.proto"}
	assert.NoError(t, c.InitDefaults())
	assert.Equal(t, []string{"parser" + separator + "test.proto"}, c.Proto)

	c.Proto = []string{"parser/[tb]est.proto"}
	assert.NoError(t, c.InitDefaults())
	assert.Equal(t, []string{"parser" + separator + "test.proto"}, c.Proto)

	c.Proto = []string{"[[[error"}
	assert.Error(t, c.InitDefaults())
}

func TestReflectionConfig(t *testing.T) {
	t.Run("nil reflection config creates default", func(t *testing.T) {
		c := &Config{
			Listen: "localhost:1234",
		}
		assert.NoError(t, c.InitDefaults())
		assert.NotNil(t, c.Reflection)
		assert.Nil(t, c.Reflection.ImportPaths)
	})

	t.Run("empty import paths", func(t *testing.T) {
		c := &Config{
			Listen: "localhost:1234",
			Reflection: &ReflectionConfig{
				ImportPaths: []string{},
			},
		}
		assert.NoError(t, c.InitDefaults())
		assert.Empty(t, c.Reflection.ImportPaths)
	})

	t.Run("with import paths", func(t *testing.T) {
		c := &Config{
			Listen: "localhost:1234",
			Reflection: &ReflectionConfig{
				ImportPaths: []string{"path1", "path2"},
			},
		}
		assert.NoError(t, c.InitDefaults())
		assert.Len(t, c.Reflection.ImportPaths, 2)
		assert.Equal(t, "path1", c.Reflection.ImportPaths[0])
		assert.Equal(t, "path2", c.Reflection.ImportPaths[1])
	})

	t.Run("nil import paths", func(t *testing.T) {
		c := &Config{
			Listen: "localhost:1234",
			Reflection: &ReflectionConfig{
				ImportPaths: nil,
			},
		}
		assert.NoError(t, c.InitDefaults())
		assert.Nil(t, c.Reflection.ImportPaths)
	})
}

func TestReflectionImportPaths(t *testing.T) {
	tests := []struct {
		name          string
		config        *Config
		expectedPaths []string
		shouldBeEmpty bool
		shouldBeNil   bool
	}{
		{
			name: "single import path",
			config: &Config{
				Listen: "localhost:1234",
				Reflection: &ReflectionConfig{
					ImportPaths: []string{"/path/to/proto"},
				},
			},
			expectedPaths: []string{"/path/to/proto"},
		},
		{
			name: "multiple import paths",
			config: &Config{
				Listen: "localhost:1234",
				Reflection: &ReflectionConfig{
					ImportPaths: []string{"/path1", "/path2", "/path3"},
				},
			},
			expectedPaths: []string{"/path1", "/path2", "/path3"},
		},
		{
			name: "empty import paths",
			config: &Config{
				Listen: "localhost:1234",
				Reflection: &ReflectionConfig{
					ImportPaths: []string{},
				},
			},
			shouldBeEmpty: true,
		},
		{
			name: "nil import paths for auto-detection",
			config: &Config{
				Listen: "localhost:1234",
				Reflection: &ReflectionConfig{
					ImportPaths: nil,
				},
			},
			shouldBeNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.InitDefaults()
			assert.NoError(t, err)

			switch {
			case tt.shouldBeNil:
				assert.Nil(t, tt.config.Reflection.ImportPaths)
			case tt.shouldBeEmpty:
				assert.Empty(t, tt.config.Reflection.ImportPaths)
			default:
				assert.Equal(t, len(tt.expectedPaths), len(tt.config.Reflection.ImportPaths))
				for i, expectedPath := range tt.expectedPaths {
					assert.Equal(t, expectedPath, tt.config.Reflection.ImportPaths[i])
				}
			}
		})
	}
}

func TestReflectionWithProtoFiles(t *testing.T) {
	t.Run("with proto files and import paths", func(t *testing.T) {
		c := &Config{
			Listen: "localhost:9090",
			Proto:  []string{"parser/test.proto", "parser/message.proto"},
			Reflection: &ReflectionConfig{
				ImportPaths: []string{"parser"},
			},
		}

		err := c.InitDefaults()
		assert.NoError(t, err)
		assert.Len(t, c.Proto, 2)
		assert.Len(t, c.Reflection.ImportPaths, 1)
	})

	t.Run("auto-detect import paths with proto files", func(t *testing.T) {
		c := &Config{
			Listen: "localhost:9090",
			Proto:  []string{"parser/test.proto"},
			Reflection: &ReflectionConfig{
				ImportPaths: nil,
			},
		}

		err := c.InitDefaults()
		assert.NoError(t, err)
		assert.Len(t, c.Proto, 1)
		assert.Nil(t, c.Reflection.ImportPaths)
	})

	t.Run("with empty proto files", func(t *testing.T) {
		c := &Config{
			Listen: "localhost:9090",
			Proto:  []string{},
			Reflection: &ReflectionConfig{
				ImportPaths: []string{"parser"},
			},
		}

		err := c.InitDefaults()
		assert.NoError(t, err)
		assert.Empty(t, c.Proto)
		assert.Len(t, c.Reflection.ImportPaths, 1)
	})
}
