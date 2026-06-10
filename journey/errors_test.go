package journey

import (
	"errors"
	"testing"
)

func TestPlanError_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  *PlanError
		want string
	}{
		{
			name: "without path",
			err:  &PlanError{Kind: "unknown_journey"},
			want: "journey: BuildPlan: unknown_journey",
		},
		{
			name: "with path and inner",
			err:  &PlanError{Kind: "cycle", Path: []string{"checkout", "api.GET /cart"}, Inner: errors.New("back edge")},
			want: "journey: BuildPlan: cycle at checkout -> api.GET /cart: back edge",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExecuteError_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  *ExecuteError
		want string
	}{
		{
			name: "without inner",
			err:  &ExecuteError{Kind: "nil_plan"},
			want: "journey: Execute: nil_plan",
		},
		{
			name: "with inner",
			err:  &ExecuteError{Kind: "internal", Inner: errors.New("panic during Execute: boom")},
			want: "journey: Execute: internal: panic during Execute: boom",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrors_Unwrap(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("sentinel")
	planErr := &PlanError{Kind: "validate", Inner: sentinel}
	if !errors.Is(planErr, sentinel) {
		t.Fatalf("errors.Is(planErr, sentinel) = false, want true")
	}

	executeErr := &ExecuteError{Kind: "internal", Inner: sentinel}
	if !errors.Is(executeErr, sentinel) {
		t.Fatalf("errors.Is(executeErr, sentinel) = false, want true")
	}
}

func TestAllowedErrorTypes_NonEmpty_And_Unique(t *testing.T) {
	t.Parallel()

	if len(AllowedErrorTypes) == 0 {
		t.Fatal("AllowedErrorTypes is empty")
	}
	seen := make(map[string]struct{}, len(AllowedErrorTypes))
	for _, typ := range AllowedErrorTypes {
		if typ == "" {
			t.Fatal("AllowedErrorTypes contains empty string")
		}
		if _, ok := seen[typ]; ok {
			t.Fatalf("AllowedErrorTypes contains duplicate %q", typ)
		}
		seen[typ] = struct{}{}
	}
	if len(AllowedErrorTypes) != 16 {
		t.Fatalf("len(AllowedErrorTypes) = %d, want 16", len(AllowedErrorTypes))
	}
}
