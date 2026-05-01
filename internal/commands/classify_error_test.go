package commands

import (
	"fmt"
	"testing"

	wkerrors "github.com/workato-devs/wk-cli-beta/internal/errors"
)

func TestClassifyError_AuthSentinels(t *testing.T) {
	authErrors := []error{
		wkerrors.ErrNoActiveProfile,
		wkerrors.ErrProfileNotFound,
		wkerrors.ErrCredentialNotFound,
		wkerrors.ErrTokenExpired,
		wkerrors.ErrProfileMismatch,
		wkerrors.ErrDuplicateTarget,
		wkerrors.ErrAPIUnauthorized,
		wkerrors.ErrAPIForbidden,
	}
	for _, sentinel := range authErrors {
		if got := classifyError(sentinel); got != wkerrors.ExitAuth {
			t.Errorf("classifyError(%v) = %d, want %d (ExitAuth)", sentinel, got, wkerrors.ExitAuth)
		}
	}
}

func TestClassifyError_ProjectSentinels(t *testing.T) {
	projectErrors := []error{
		wkerrors.ErrNotInProject,
		wkerrors.ErrProjectExists,
		wkerrors.ErrNestedProject,
	}
	for _, sentinel := range projectErrors {
		if got := classifyError(sentinel); got != wkerrors.ExitNoProject {
			t.Errorf("classifyError(%v) = %d, want %d (ExitNoProject)", sentinel, got, wkerrors.ExitNoProject)
		}
	}
}

func TestClassifyError_APISentinels(t *testing.T) {
	apiErrors := []error{
		wkerrors.ErrAPINotFound,
		wkerrors.ErrAPIRateLimit,
		wkerrors.ErrAPIServer,
	}
	for _, sentinel := range apiErrors {
		if got := classifyError(sentinel); got != wkerrors.ExitAPI {
			t.Errorf("classifyError(%v) = %d, want %d (ExitAPI)", sentinel, got, wkerrors.ExitAPI)
		}
	}
}

func TestClassifyError_PluginSentinels(t *testing.T) {
	pluginErrors := []error{
		wkerrors.ErrPluginNotFound,
		wkerrors.ErrPluginTimeout,
		wkerrors.ErrPluginProtocol,
	}
	for _, sentinel := range pluginErrors {
		if got := classifyError(sentinel); got != wkerrors.ExitPlugin {
			t.Errorf("classifyError(%v) = %d, want %d (ExitPlugin)", sentinel, got, wkerrors.ExitPlugin)
		}
	}
}

func TestClassifyError_UnknownError(t *testing.T) {
	err := fmt.Errorf("something unexpected")
	if got := classifyError(err); got != wkerrors.ExitGeneral {
		t.Errorf("classifyError(unknown) = %d, want %d (ExitGeneral)", got, wkerrors.ExitGeneral)
	}
}

func TestClassifyError_WrappedWithSentinel(t *testing.T) {
	err := wkerrors.WithSentinel(wkerrors.ErrPluginNotFound, "plugin %q is not installed", "foo")
	if got := classifyError(err); got != wkerrors.ExitPlugin {
		t.Errorf("classifyError(WithSentinel) = %d, want %d (ExitPlugin)", got, wkerrors.ExitPlugin)
	}
}

func TestClassifyError_FmtWrappedSentinel(t *testing.T) {
	err := fmt.Errorf("context: %w", wkerrors.ErrAPINotFound)
	if got := classifyError(err); got != wkerrors.ExitAPI {
		t.Errorf("classifyError(fmt.Errorf %%w) = %d, want %d (ExitAPI)", got, wkerrors.ExitAPI)
	}
}
