package grpc

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	t.Run("nil reflection config", func(t *testing.T) {
		c := &Config{}
		assert.False(t, c.EnableReflection())
		nilDescriptors := c.GetDescriptorSets()
		assert.Nil(t, nilDescriptors)
	})

	t.Run("reflection disabled", func(t *testing.T) {
		c := &Config{
			Reflection: &ReflectionConfig{
				Enabled:        false,
				DescriptorSets: []string{"test1.descriptor", "test2.descriptor"},
			},
		}
		assert.False(t, c.EnableReflection())
		descriptors := c.GetDescriptorSets()
		assert.NotNil(t, descriptors)
		assert.Len(t, descriptors, 2)
		assert.Equal(t, []string{"test1.descriptor", "test2.descriptor"}, descriptors)
	})

	t.Run("reflection enabled with descriptors", func(t *testing.T) {
		c := &Config{
			Reflection: &ReflectionConfig{
				Enabled:        true,
				DescriptorSets: []string{"test1.descriptor", "test2.descriptor"},
			},
		}
		assert.True(t, c.EnableReflection())
		descriptors := c.GetDescriptorSets()
		require.Len(t, descriptors, 2)
		assert.Equal(t, "test1.descriptor", descriptors[0])
		assert.Equal(t, "test2.descriptor", descriptors[1])
	})

	t.Run("reflection enabled without descriptors", func(t *testing.T) {
		c := &Config{
			Reflection: &ReflectionConfig{
				Enabled:        true,
				DescriptorSets: []string{},
			},
		}
		assert.True(t, c.EnableReflection())
		descriptors := c.GetDescriptorSets()
		assert.Empty(t, descriptors)
	})

	t.Run("reflection enabled with nil descriptors", func(t *testing.T) {
		c := &Config{
			Reflection: &ReflectionConfig{
				Enabled: true,
			},
		}
		assert.True(t, c.EnableReflection())
		descriptors := c.GetDescriptorSets()
		assert.Nil(t, descriptors)
	})
}
