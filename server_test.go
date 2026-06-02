package grpc

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestStatusDetails_NoDetails_Nil(t *testing.T) {
	s := status.New(codes.InvalidArgument, "bad arg")
	assert.Nil(t, statusDetails(s))
}

func TestStatusDetails_NonStatusError_Nil(t *testing.T) {
	s, ok := status.FromError(errors.New("boom"))
	require.False(t, ok)
	assert.Nil(t, statusDetails(s), "a non-status error carries no details")
}

func TestStatusDetails_SingleDetail(t *testing.T) {
	s, err := status.New(codes.InvalidArgument, "bad arg").WithDetails(&errdetails.BadRequest{
		FieldViolations: []*errdetails.BadRequest_FieldViolation{
			{Field: "email", Description: "is required"},
		},
	})
	require.NoError(t, err)

	got := statusDetails(s)
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "google.rpc.BadRequest", "should name the detail type")
	assert.Contains(t, got[0], "email", "should include the field value")
	assert.Contains(t, got[0], "is required")
}

func TestStatusDetails_MultipleDetails(t *testing.T) {
	s, err := status.New(codes.FailedPrecondition, "nope").WithDetails(
		&errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{{Field: "x", Description: "y"}}},
		&errdetails.ErrorInfo{Reason: "QUOTA", Domain: "example.com"},
	)
	require.NoError(t, err)

	got := statusDetails(s)
	require.Len(t, got, 2)
	joined := got[0] + " | " + got[1]
	assert.Contains(t, joined, "google.rpc.BadRequest")
	assert.Contains(t, joined, "google.rpc.ErrorInfo")
	assert.Contains(t, joined, "QUOTA")
}

func TestStatusDetails_SkipsNonGoogleRpc(t *testing.T) {
	// Handlers can attach arbitrary (and potentially large) details; only the
	// standard google.rpc.* error details should be logged.
	s, err := status.New(codes.Internal, "boom").WithDetails(&wrapperspb.StringValue{Value: "arbitrary payload"})
	require.NoError(t, err)
	assert.Empty(t, statusDetails(s), "non-google.rpc details must be skipped")
}

func TestStatusDetails_MixedKeepsOnlyGoogleRpc(t *testing.T) {
	s, err := status.New(codes.InvalidArgument, "bad").WithDetails(
		&wrapperspb.StringValue{Value: "arbitrary payload"},
		&errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{{Field: "email", Description: "is required"}}},
	)
	require.NoError(t, err)

	got := statusDetails(s)
	require.Len(t, got, 1, "only the google.rpc detail should be kept")
	assert.Contains(t, got[0], "google.rpc.BadRequest")
	assert.NotContains(t, got[0], "StringValue")
}
