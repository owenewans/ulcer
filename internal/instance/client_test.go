package instance

import (
	"errors"
	"io"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIdentityRejected(t *testing.T) {
	for _, code := range []codes.Code{codes.Unauthenticated, codes.PermissionDenied} {
		if !identityRejected(status.Error(code, "rejected")) {
			t.Errorf("code %s was not permanent", code)
		}
	}
	for _, err := range []error{nil, errors.New("network"), status.Error(codes.Aborted, "retry")} {
		if identityRejected(err) {
			t.Errorf("error %v was incorrectly permanent", err)
		}
	}
}

func TestReceiveErrorPreservesGRPCStatus(t *testing.T) {
	rejected := status.Error(codes.PermissionDenied, "deleted")
	if got := receiveError(rejected); status.Code(got) != codes.PermissionDenied {
		t.Fatalf("receive error code = %s, want PermissionDenied", status.Code(got))
	}
	if got := receiveError(io.EOF); got == nil || status.Code(got) != codes.Unknown {
		t.Fatalf("EOF conversion = %v", got)
	}
}
