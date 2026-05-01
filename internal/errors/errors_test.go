package errors

import (
	"errors"
	"testing"
)

func TestWithSentinel_ErrorReturnsUserMessage(t *testing.T) {
	err := WithSentinel(ErrPluginNotFound, "plugin %q is not installed", "foo")
	want := `plugin "foo" is not installed`
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestWithSentinel_IsMatchesSentinel(t *testing.T) {
	err := WithSentinel(ErrPluginNotFound, "plugin %q is not installed", "foo")
	if !errors.Is(err, ErrPluginNotFound) {
		t.Error("errors.Is should match ErrPluginNotFound")
	}
}

func TestWithSentinel_IsDoesNotMatchOtherSentinel(t *testing.T) {
	err := WithSentinel(ErrPluginNotFound, "some message")
	if errors.Is(err, ErrPluginTimeout) {
		t.Error("errors.Is should not match ErrPluginTimeout")
	}
}

func TestWithSentinel_UnwrapReturnsSentinel(t *testing.T) {
	err := WithSentinel(ErrProfileMismatch, "profiles differ")
	unwrapped := errors.Unwrap(err)
	if unwrapped != ErrProfileMismatch {
		t.Errorf("Unwrap() = %v, want ErrProfileMismatch", unwrapped)
	}
}

func TestWithSentinel_SentinelTextNotInError(t *testing.T) {
	err := WithSentinel(ErrPluginNotFound, "plugin %q is not installed", "bar")
	msg := err.Error()
	sentinel := ErrPluginNotFound.Error()
	if contains(msg, sentinel) {
		t.Errorf("Error() %q should not contain sentinel text %q", msg, sentinel)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
