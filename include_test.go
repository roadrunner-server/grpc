package grpc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIncludesParser(t *testing.T) {
	dc := "--descriptor_set_out=foo"
	fi := "-Ifoo"
	ui := []string{"message.proto", "conf/a/b/super.proto"}
	proto := "foo.proto"

	require.Equal(t, []string{"--descriptor_set_out=foo", "-Ifoo", "-Imessage.proto", "-Iconf/a/b/super.proto", "foo.proto"}, parseInclude(dc, fi, ui, proto))
}

func TestIncludesParser2(t *testing.T) {
	dc := "--descriptor_set_out=foo"
	fi := "-Ifoo"
	proto := "foo.proto"

	require.Equal(t, []string{"--descriptor_set_out=foo", "-Ifoo", "foo.proto"}, parseInclude(dc, fi, nil, proto))
}
