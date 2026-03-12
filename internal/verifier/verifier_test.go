package verifier_test

import (
	"context"
	"testing"

	"github.com/runger/attest/internal/state"
	"github.com/runger/attest/internal/verifier"
)

func TestVerifyPassesWithValidEvidence(t *testing.T) {
	task := &state.Task{
		TaskID:           "task-1",
		RequirementIDs:   []string{"AT-FR-001"},
		RequiredEvidence: []string{"quality_gate_pass"},
	}

	report := &state.CompletionReport{
		TaskID:    "task-1",
		AttemptID: "attempt-1",
		CommandResults: []state.CommandResult{
			{CommandID: "cmd-1", Command: "go test", ExitCode: 0, Required: true},
		},
	}

	result, err := verifier.Verify(context.Background(), task, report, nil, t.TempDir())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !result.Pass {
		t.Errorf("expected pass, got fail with findings: %v", result.BlockingFindings)
	}
}

// AT-TS-003: A task with no independent verifier evidence cannot close.
func TestVerifyFailsWithNoReport(t *testing.T) {
	task := &state.Task{
		TaskID:           "task-1",
		RequirementIDs:   []string{"AT-FR-001"},
		RequiredEvidence: []string{"test_pass"},
	}

	report := &state.CompletionReport{
		TaskID:    "task-1",
		AttemptID: "attempt-1",
		// No command results — missing evidence.
	}

	result, err := verifier.Verify(context.Background(), task, report, nil, t.TempDir())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if result.Pass {
		t.Error("expected fail for missing evidence, got pass")
	}
	if len(result.BlockingFindings) == 0 {
		t.Error("expected blocking findings")
	}
}

func TestVerifyFailsWithNoRequirementIDs(t *testing.T) {
	task := &state.Task{
		TaskID:         "task-1",
		RequirementIDs: []string{}, // No linked requirements.
	}

	report := &state.CompletionReport{
		TaskID:    "task-1",
		AttemptID: "attempt-1",
	}

	result, err := verifier.Verify(context.Background(), task, report, nil, t.TempDir())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if result.Pass {
		t.Error("expected fail for no requirement linkage, got pass")
	}
}

func TestVerifyScopeViolation(t *testing.T) {
	task := &state.Task{
		TaskID:         "task-1",
		RequirementIDs: []string{"AT-FR-001"},
		Scope: state.TaskScope{
			OwnedPaths: []string{"internal/engine/"},
		},
		RequiredEvidence: []string{"quality_gate_pass"},
	}

	report := &state.CompletionReport{
		TaskID:    "task-1",
		AttemptID: "attempt-1",
		ChangedFiles: []string{
			"cmd/attest/main.go", // Outside scope.
		},
	}

	result, err := verifier.Verify(context.Background(), task, report, nil, t.TempDir())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if result.Pass {
		t.Error("expected fail for scope violation, got pass")
	}
}

// AT-TS-025: A task whose implementation fails the quality gate cannot proceed to model review.
func TestVerifyQualityGateBlocksReview(t *testing.T) {
	task := &state.Task{
		TaskID:           "task-1",
		RequirementIDs:   []string{"AT-FR-001"},
		RequiredEvidence: []string{"quality_gate_pass"},
	}

	report := &state.CompletionReport{
		TaskID:    "task-1",
		AttemptID: "attempt-1",
	}

	gate := &state.QualityGate{
		Command:        "false", // Always fails.
		TimeoutSeconds: 10,
		Required:       true,
	}

	result, err := verifier.Verify(context.Background(), task, report, gate, t.TempDir())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if result.Pass {
		t.Error("expected fail when quality gate fails")
	}

	// Verify the failure is a quality gate finding.
	found := false
	for _, f := range result.BlockingFindings {
		if f.Category == "quality_gate" {
			found = true
		}
	}
	if !found {
		t.Error("expected quality_gate category in findings")
	}
}
