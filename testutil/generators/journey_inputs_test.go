package generators

import (
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/journey"
	"pgregory.net/rapid"
)

func TestValidPlan_GeneratesInvariantRespectingValues(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		plan := ValidPlan().Draw(t, "plan")
		if plan == nil {
			t.Fatal("ValidPlan produced nil")
			return
		}
		if plan.JourneyName == "" {
			t.Fatal("ValidPlan produced empty JourneyName")
		}
		if plan.Root == nil {
			t.Fatal("ValidPlan produced nil Root")
		}
		assertValidJourneyNode(t, plan.Root)
	})
}

func TestValidNode_GeneratesInvariantRespectingValues(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		node := ValidNode().Draw(t, "node")
		if node == nil {
			t.Fatal("ValidNode produced nil")
			return
		}
		assertValidJourneyNode(t, node)
	})
}

func TestValidEngineOutcome_GeneratesInvariantRespectingValues(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		outcome := ValidEngineOutcome().Draw(t, "outcome")
		assertValidEngineOutcome(t, outcome)
	})
}

func assertValidJourneyNode(t *rapid.T, node *journey.Node) {
	t.Helper()

	if len(node.Children) > 0 && len(node.Parallel) > 0 {
		t.Fatalf("node has both Children and Parallel: %+v", node)
	}
	virtual := node.Service == nil
	if virtual {
		if node.Operation != "" {
			t.Fatalf("virtual node Operation = %q, want empty", node.Operation)
		}
		if len(node.Children) == 0 && len(node.Parallel) == 0 {
			t.Fatalf("virtual node has neither Children nor Parallel")
		}
	} else {
		if node.Operation == "" {
			t.Fatalf("concrete node Operation is empty")
		}
		if node.Service.Operations[node.Operation] == nil {
			t.Fatalf("operation %q is not present in service %s", node.Operation, node.Service.Name)
		}
	}
	for _, child := range node.Children {
		if child == nil {
			t.Fatal("node has nil child")
		}
		assertValidJourneyNode(t, child)
	}
	for _, child := range node.Parallel {
		if child == nil {
			t.Fatal("node has nil parallel child")
		}
		assertValidJourneyNode(t, child)
	}
}

func assertValidEngineOutcome(t *rapid.T, outcome journey.Outcome) {
	t.Helper()

	if outcome.Success && outcome.ErrorType != "" {
		t.Fatalf("successful outcome has ErrorType %q", outcome.ErrorType)
	}
	if !outcome.Success && !outcome.Cascaded && outcome.ErrorType == "" {
		t.Fatalf("failed non-cascaded outcome has empty ErrorType")
	}
	if outcome.Cascaded && outcome.Latency > time.Millisecond {
		t.Fatalf("cascaded outcome Latency = %s, want near zero", outcome.Latency)
	}
	if (outcome.DefaultUsed || outcome.SilentlySucceeded) && !outcome.Success {
		t.Fatalf("default/silent outcome is not successful: %+v", outcome)
	}
	if outcome.ErrorType != "" && !containsAllowedJourneyError(outcome.ErrorType) {
		t.Fatalf("ErrorType = %q is not allowed", outcome.ErrorType)
	}
}

func containsAllowedJourneyError(errorType string) bool {
	for _, allowed := range journey.AllowedErrorTypes {
		if errorType == allowed {
			return true
		}
	}
	return false
}
