package compiler_test

import (
	"testing"

	"github.com/runger/attest/internal/compiler"
	"github.com/runger/attest/internal/state"
)

func TestCompileDeterministic(t *testing.T) {
	// AT-TS-001: Preparing the same approved run artifact twice yields the same initial tasks.
	artifact := &state.RunArtifact{
		SchemaVersion: "0.1",
		RunID:         "test-run",
		Requirements: []state.Requirement{
			{ID: "AT-FR-002", Text: "Second requirement"},
			{ID: "AT-FR-001", Text: "First requirement"},
			{ID: "AT-FR-003", Text: "Third requirement"},
		},
	}

	result1, err := compiler.Compile(artifact)
	if err != nil {
		t.Fatalf("Compile 1: %v", err)
	}

	result2, err := compiler.Compile(artifact)
	if err != nil {
		t.Fatalf("Compile 2: %v", err)
	}

	if len(result1.Tasks) != len(result2.Tasks) {
		t.Fatalf("task count differs: %d vs %d", len(result1.Tasks), len(result2.Tasks))
	}

	for i := range result1.Tasks {
		if result1.Tasks[i].TaskID != result2.Tasks[i].TaskID {
			t.Errorf("task %d ID differs: %q vs %q", i, result1.Tasks[i].TaskID, result2.Tasks[i].TaskID)
		}
		if result1.Tasks[i].Title != result2.Tasks[i].Title {
			t.Errorf("task %d title differs", i)
		}
	}
}

func TestCompileSortedOutput(t *testing.T) {
	artifact := &state.RunArtifact{
		Requirements: []state.Requirement{
			{ID: "AT-FR-003", Text: "C"},
			{ID: "AT-FR-001", Text: "A"},
			{ID: "AT-FR-002", Text: "B"},
		},
	}

	result, err := compiler.Compile(artifact)
	if err != nil {
		t.Fatal(err)
	}

	// Tasks should be sorted by requirement ID.
	if result.Tasks[0].TaskID != "task-at-fr-001" {
		t.Errorf("first task = %q, want task-at-fr-001", result.Tasks[0].TaskID)
	}
	if result.Tasks[1].TaskID != "task-at-fr-002" {
		t.Errorf("second task = %q, want task-at-fr-002", result.Tasks[1].TaskID)
	}
}

func TestCompileCoverage(t *testing.T) {
	artifact := &state.RunArtifact{
		Requirements: []state.Requirement{
			{ID: "AT-FR-001", Text: "Req one"},
			{ID: "AT-FR-002", Text: "Req two"},
		},
	}

	result, err := compiler.Compile(artifact)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Coverage) != 2 {
		t.Fatalf("coverage count = %d, want 2", len(result.Coverage))
	}

	// Check no unassigned requirements.
	unassigned := compiler.CheckCoverage(artifact, result.Coverage)
	if len(unassigned) != 0 {
		t.Errorf("unexpected unassigned: %v", unassigned)
	}
}

func TestCheckCoverageDetectsGaps(t *testing.T) {
	artifact := &state.RunArtifact{
		Requirements: []state.Requirement{
			{ID: "AT-FR-001", Text: "Covered"},
			{ID: "AT-FR-002", Text: "Not covered"},
		},
	}

	// Only AT-FR-001 is covered.
	coverage := []state.RequirementCoverage{
		{RequirementID: "AT-FR-001", CoveringTaskIDs: []string{"task-1"}},
	}

	unassigned := compiler.CheckCoverage(artifact, coverage)
	if len(unassigned) != 1 || unassigned[0] != "AT-FR-002" {
		t.Errorf("unassigned = %v, want [AT-FR-002]", unassigned)
	}
}

func TestCompileNilArtifact(t *testing.T) {
	_, err := compiler.Compile(nil)
	if err == nil {
		t.Fatal("expected error for nil artifact")
	}
}

func TestCompileEmptyRequirements(t *testing.T) {
	_, err := compiler.Compile(&state.RunArtifact{})
	if err == nil {
		t.Fatal("expected error for empty requirements")
	}
}
