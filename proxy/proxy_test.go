package proxy

import (
	stderr "errors"
	"testing"

	"github.com/roadrunner-server/errors"
	"github.com/stretchr/testify/require"
)

func TestWrapError(t *testing.T) {
	msg := "7|:|Unauthorized access `index`|:|\n(type.googleapis.com/google.rpc.ErrorInfo\u0012_\n\u0010PermissionDenied\u0012#app.ServiceName\u001a&\n\u0007message\u0012\u001bUnauthorized access `index`"
	err := stderr.New(msg)
	newErr := wrapError(err)
	require.Equal(t, "rpc error: code = PermissionDenied desc = Unauthorized access `index`", newErr.Error())
}

func TestRRErrorPackage(t *testing.T) {
	msg := "7|:|Unauthorized access `index`|:|\n(type.googleapis.com/google.rpc.ErrorInfo\u0012_\n\u0010PermissionDenied\u0012#app.ServiceName\u001a&\n\u0007message\u0012\u001bUnauthorized access `index`"
	const op1 = errors.Op("foo_op")
	const op2 = errors.Op("foo2_op")
	err := errors.E(errors.Str(msg), errors.SkipFile)
	err = errors.E(err, op1, errors.Undefined)
	err = errors.E(err, op2, errors.Retry)

	retErr := wrapError(err)
	require.Equal(t, "rpc error: code = PermissionDenied desc = Unauthorized access `index`", retErr.Error())
}
