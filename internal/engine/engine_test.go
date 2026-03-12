package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/runger/attest/internal/engine"
	"github.com/runger/attest/internal/state"
)

func writeTestSpec(t *testing.T, dir string) string {
	t.Helper()
	spec := filepath.Join(dir, "test-spec.md")
	content := `# Test Spec

- **AT-FR-001**: The system must ingest specs.
- **AT-FR-002**: The system must compile tasks.
- **AT-TS-001**: Deterministic compilation scenario.
`
	if err := os.WriteFile(spec, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestPrepareAndCompile(t *testing.T) {
	dir := t.TempDir()
	specPath := writeTestSpec(t, dir)

	runDir := state.NewRunDir(dir, "placeholder")
	eng := engine.New(runDir, dir)

	ctx := context.Background()

	// Prepare.
	artifact, err := eng.Prepare(ctx, []string{specPath})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if len(artifact.Requirements) != 3 {
		t.Fatalf("got %d requirements, want 3", len(artifact.Requirements))
	}
	if artifact.RunID == "" {
		t.Error("run ID is empty")
	}

	// Approve.
	if err := eng.Approve(ctx); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// Compile.
	result, err := eng.Compile(ctx)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(result.Tasks) != 3 {
		t.Fatalf("got %d tasks, want 3", len(result.Tasks))
	}

	// All tasks should be pending.
	for _, task := range result.Tasks {
		if task.Status != state.TaskPending {
			t.Errorf("task %s status = %s, want pending", task.TaskID, task.Status)
		}
	}
}

// AT-TS-001: Preparing the same artifact twice yields the same tasks.
func TestCompileDeterminism(t *testing.T) {
	dir := t.TempDir()
	specPath := writeTestSpec(t, dir)
	ctx := context.Background()

	// First run.
	runDir1 := state.NewRunDir(dir, "placeholder1")
	eng1 := engine.New(runDir1, dir)
	if _, err := eng1.Prepare(ctx, []string{specPath}); err != nil {
		t.Fatal(err)
	}
	if err := eng1.Approve(ctx); err != nil {
		t.Fatal(err)
	}
	result1, err := eng1.Compile(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Second run.
	runDir2 := state.NewRunDir(dir, "placeholder2")
	eng2 := engine.New(runDir2, dir)
	if _, err := eng2.Prepare(ctx, []string{specPath}); err != nil {
		t.Fatal(err)
	}
	if err := eng2.Approve(ctx); err != nil {
		t.Fatal(err)
	}
	result2, err := eng2.Compile(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Task IDs must be identical.
	if len(result1.Tasks) != len(result2.Tasks) {
		t.Fatalf("task count differs: %d vs %d", len(result1.Tasks), len(result2.Tasks))
	}
	for i := range result1.Tasks {
		if result1.Tasks[i].TaskID != result2.Tasks[i].TaskID {
			t.Errorf("task %d ID differs: %q vs %q", i, result1.Tasks[i].TaskID, result2.Tasks[i].TaskID)
		}
	}
}

func TestGetPendingTasks(t *testing.T) {
	dir := t.TempDir()
	specPath := writeTestSpec(t, dir)
	ctx := context.Background()

	runDir := state.NewRunDir(dir, "placeholder")
	eng := engine.New(runDir, dir)

	if _, err := eng.Prepare(ctx, []string{specPath}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Approve(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Compile(ctx); err != nil {
		t.Fatal(err)
	}

	pending, err := eng.GetPendingTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 3 {
		t.Fatalf("got %d pending tasks, want 3", len(pending))
	}
}

func TestUpdateTaskStatus(t *testing.T) {
	dir := t.TempDir()
	specPath := writeTestSpec(t, dir)
	ctx := context.Background()

	runDir := state.NewRunDir(dir, "placeholder")
	eng := engine.New(runDir, dir)

	if _, err := eng.Prepare(ctx, []string{specPath}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Approve(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := eng.Compile(ctx)
	if err != nil {
		t.Fatal(err)
	}

	taskID := result.Tasks[0].TaskID
	if err := eng.UpdateTaskStatus(taskID, state.TaskDone, "verified"); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}

	// Verify the change persisted.
	tasks, err := eng.RunDir.ReadTasks()
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if task.TaskID == taskID && task.Status != state.TaskDone {
			t.Errorf("task %s status = %s after update, want done", taskID, task.Status)
		}
	}
}
