package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestWriteJSON_Proto(t *testing.T) {
	w := httptest.NewRecorder()
	if err := WriteJSON(w, &pbuser.UserProfile{UserId: "u1"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("expected JSON content type")
	}
	if !strings.Contains(w.Body.String(), "u1") {
		t.Errorf("expected u1 in body: %s", w.Body.String())
	}
}

func TestWriteJSON_Native(t *testing.T) {
	w := httptest.NewRecorder()
	if err := WriteJSON(w, map[string]int{"n": 7}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(w.Body.String(), "7") {
		t.Errorf("expected 7 in body: %s", w.Body.String())
	}
}

func TestWriteError_GRPCCodeMapping(t *testing.T) {
	cases := []struct {
		code codes.Code
		want int
	}{
		{codes.NotFound, http.StatusNotFound},
		{codes.InvalidArgument, http.StatusBadRequest},
		{codes.PermissionDenied, http.StatusForbidden},
		{codes.Unauthenticated, http.StatusUnauthorized},
		{codes.AlreadyExists, http.StatusConflict},
		{codes.Unimplemented, http.StatusNotImplemented},
		{codes.Unavailable, http.StatusServiceUnavailable},
		{codes.FailedPrecondition, http.StatusPreconditionFailed},
		{codes.ResourceExhausted, http.StatusTooManyRequests},
		{codes.DeadlineExceeded, http.StatusGatewayTimeout},
		{codes.Internal, http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.code.String(), func(t *testing.T) {
			w := httptest.NewRecorder()
			WriteError(w, status.Error(c.code, "boom"))
			if w.Code != c.want {
				t.Errorf("code %v -> http %d, want %d", c.code, w.Code, c.want)
			}
			if !strings.Contains(w.Body.String(), "boom") {
				t.Errorf("expected message in body: %s", w.Body.String())
			}
		})
	}
}

func TestWriteError_CustomError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, statusError(http.StatusBadRequest, "bad input"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "bad input") {
		t.Error("expected custom message")
	}
}

func TestWriteError_NonGRPC(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, errors.New("plain error"))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestCustomError_Error(t *testing.T) {
	e := &CustomError{HTTPCode: 418, Msg: "teapot"}
	if e.Error() != "teapot" {
		t.Errorf("Error() = %q", e.Error())
	}
}
