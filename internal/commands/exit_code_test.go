package commands

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestExitCodeFromResult_NonZero(t *testing.T) {
	result := json.RawMessage(`{"exit_code": 2, "message": "invalid input"}`)
	err := exitCodeFromResult(result)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitCodeError, got %T", err)
	}
	if exitErr.Code != 2 {
		t.Errorf("Code = %d, want 2", exitErr.Code)
	}
}

func TestExitCodeFromResult_Zero(t *testing.T) {
	result := json.RawMessage(`{"exit_code": 0, "message": "ok"}`)
	if err := exitCodeFromResult(result); err != nil {
		t.Errorf("expected nil for exit_code 0, got %v", err)
	}
}

func TestExitCodeFromResult_Missing(t *testing.T) {
	result := json.RawMessage(`{"message": "legacy plugin"}`)
	if err := exitCodeFromResult(result); err != nil {
		t.Errorf("expected nil when exit_code absent, got %v", err)
	}
}

func TestExitCodeFromResult_NotObject(t *testing.T) {
	result := json.RawMessage(`"plain string"`)
	if err := exitCodeFromResult(result); err != nil {
		t.Errorf("expected nil for non-object result, got %v", err)
	}
}

func TestExitCodeError_ErrorString(t *testing.T) {
	err := ExitCodeError{Code: 1}
	if got := err.Error(); got != "exit code 1" {
		t.Errorf("Error() = %q, want %q", got, "exit code 1")
	}
}
