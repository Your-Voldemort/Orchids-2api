package errors

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *AppError
		expected string
	}{
		{
			name:     "without cause",
			err:      ErrInvalidRequest,
			expected: "[invalid_request_error] 请求格式无效",
		},
		{
			name:     "with cause",
			err:      ErrInvalidRequest.WithCause(errors.New("json decode failed")),
			expected: "[invalid_request_error] 请求格式无效: json decode failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("AppError.Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestAppError_WithCause(t *testing.T) {
	cause := errors.New("test cause")
	err := ErrInvalidRequest.WithCause(cause)

	if err.Cause != cause {
		t.Errorf("WithCause() cause = %v, want %v", err.Cause, cause)
	}
	if err.Code != ErrInvalidRequest.Code {
		t.Errorf("WithCause() code = %v, want %v", err.Code, ErrInvalidRequest.Code)
	}
	if err.Message != ErrInvalidRequest.Message {
		t.Errorf("WithCause() message = %v, want %v", err.Message, ErrInvalidRequest.Message)
	}
}

func TestAppError_WithMessage(t *testing.T) {
	newMsg := "自定义错误消息"
	err := ErrInvalidRequest.WithMessage(newMsg)

	if err.Message != newMsg {
		t.Errorf("WithMessage() message = %v, want %v", err.Message, newMsg)
	}
	if err.Code != ErrInvalidRequest.Code {
		t.Errorf("WithMessage() code = %v, want %v", err.Code, ErrInvalidRequest.Code)
	}
}

func TestAppError_ToJSON(t *testing.T) {
	err := ErrInvalidRequest
	json := string(err.ToJSON())

	if json == "" {
		t.Error("ToJSON() returned empty string")
	}
	if !contains(json, `"type":"error"`) {
		t.Errorf("ToJSON() missing type field: %s", json)
	}
	if !contains(json, `"type":"invalid_request_error"`) {
		t.Errorf("ToJSON() missing error type: %s", json)
	}
}

func TestAppError_WriteResponse(t *testing.T) {
	err := ErrInvalidRequest
	w := httptest.NewRecorder()

	err.WriteResponse(w)

	if w.Code != http.StatusBadRequest {
		t.Errorf("WriteResponse() status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("WriteResponse() Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestAppError_Unwrap(t *testing.T) {
	cause := errors.New("wrapped error")
	err := ErrInvalidRequest.WithCause(cause)

	if unwrapped := err.Unwrap(); unwrapped != cause {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
	}
}

func TestNew(t *testing.T) {
	err := New("custom_code", "custom message", http.StatusTeapot)

	if err.Code != "custom_code" {
		t.Errorf("New() code = %v, want %v", err.Code, "custom_code")
	}
	if err.Message != "custom message" {
		t.Errorf("New() message = %v, want %v", err.Message, "custom message")
	}
	if err.HTTPStatus != http.StatusTeapot {
		t.Errorf("New() status = %v, want %v", err.HTTPStatus, http.StatusTeapot)
	}
}

func TestWrap(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		if got := Wrap(nil, ErrInvalidRequest); got != nil {
			t.Errorf("Wrap(nil) = %v, want nil", got)
		}
	})

	t.Run("with error", func(t *testing.T) {
		cause := errors.New("test error")
		got := Wrap(cause, ErrInvalidRequest)
		if got == nil {
			t.Fatal("Wrap() returned nil for non-nil error")
		}
		if got.Cause != cause {
			t.Errorf("Wrap() cause = %v, want %v", got.Cause, cause)
		}
	})
}

func TestIs(t *testing.T) {
	t.Run("same error", func(t *testing.T) {
		if !Is(ErrInvalidRequest, ErrInvalidRequest) {
			t.Error("Is() = false, want true for same error")
		}
	})

	t.Run("wrapped error", func(t *testing.T) {
		wrapped := ErrInvalidRequest.WithCause(errors.New("cause"))
		if !Is(wrapped, ErrInvalidRequest) {
			t.Error("Is() = false, want true for wrapped error")
		}
	})

	t.Run("different error", func(t *testing.T) {
		if Is(ErrInvalidRequest, ErrUnauthorized) {
			t.Error("Is() = true, want false for different errors")
		}
	})

	t.Run("non-app error", func(t *testing.T) {
		if Is(errors.New("standard error"), ErrInvalidRequest) {
			t.Error("Is() = true, want false for non-app error")
		}
	})
}

func TestGetHTTPStatus(t *testing.T) {
	t.Run("app error", func(t *testing.T) {
		if got := GetHTTPStatus(ErrInvalidRequest); got != http.StatusBadRequest {
			t.Errorf("GetHTTPStatus() = %d, want %d", got, http.StatusBadRequest)
		}
	})

	t.Run("standard error", func(t *testing.T) {
		if got := GetHTTPStatus(errors.New("test")); got != http.StatusInternalServerError {
			t.Errorf("GetHTTPStatus() = %d, want %d", got, http.StatusInternalServerError)
		}
	})
}

func TestGetCode(t *testing.T) {
	t.Run("app error", func(t *testing.T) {
		if got := GetCode(ErrInvalidRequest); got != CodeInvalidRequest {
			t.Errorf("GetCode() = %q, want %q", got, CodeInvalidRequest)
		}
	})

	t.Run("standard error", func(t *testing.T) {
		if got := GetCode(errors.New("test")); got != CodeInternalError {
			t.Errorf("GetCode() = %q, want %q", got, CodeInternalError)
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
