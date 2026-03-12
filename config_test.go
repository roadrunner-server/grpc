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

	c.Proto = []string{"parser/test_nested/**/*.proto"}
	assert.NoError(t, c.InitDefaults())
	assert.Equal(t, []string{
		"parser" + separator + "test_nested" + separator + "message.proto",
		"parser" + separator + "test_nested" + separator + "pong.proto",
		"parser" + separator + "test_nested" + separator + "test_import.proto",
		"parser" + separator + "test_nested" + separator + "sub" + separator + "deep.proto",
	}, c.Proto)

	c.Proto = []string{"parser/{message,pong}.proto"}
	assert.NoError(t, c.InitDefaults())
	assert.Equal(t, []string{
		"parser" + separator + "message.proto",
		"parser" + separator + "pong.proto",
	}, c.Proto)

	c.Proto = []string{"parser/{*_import.proto,pong.proto}"}
	assert.NoError(t, c.InitDefaults())
	assert.Equal(t, []string{
		"parser" + separator + "pong.proto",
		"parser" + separator + "test_import.proto",
	}, c.Proto)

	c.Proto = []string{"[[[error"}
	assert.Error(t, c.InitDefaults())
}
