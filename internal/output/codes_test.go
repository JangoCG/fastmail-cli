package output

import (
	"errors"
	"fmt"
	"testing"

	"github.com/JangoCG/fastmail-cli/internal/apierr"
)

func TestExitCodeForEveryCodeAnErrorCanCarry(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{apierr.ErrUsage("bad"), ExitUsage},
		{apierr.ErrUsageHint("bad", "try this"), ExitUsage},
		{apierr.ErrNotFound("topic", "3011"), ExitNotFound},
		{apierr.ErrAuth("no"), ExitAuth},
		{apierr.ErrForbidden("no"), ExitForbidden},
		{apierr.ErrRateLimit(30), ExitRateLimit},
		{apierr.ErrNetwork(nil), ExitNetwork},
		{apierr.ErrAPI(500, "oops"), ExitAPI},
		{apierr.ErrAmbiguous("contact", nil), ExitAmbiguous},
		{apierr.ErrValidation(422, "blank", "", nil), ExitUsage},
		{apierr.ErrConflict(409, "exists", "", nil), ExitUsage},
		{&apierr.Error{Code: "upgrade_required", Message: "build too old"}, ExitUsage},
		// Something that never went through apierr at all — a bare error out of the
		// standard library or a dependency still has to exit with something.
		{errors.New("a plain error nobody wrapped"), ExitUsage},
		{fmt.Errorf("wrapped: %w", apierr.ErrNotFound("topic", "3011")), ExitNotFound},
	}

	for _, tt := range tests {
		if got := ExitCodeFor(tt.err); got != tt.want {
			t.Errorf("ExitCodeFor(%v) = %d, want %d", tt.err, got, tt.want)
		}
	}
}
