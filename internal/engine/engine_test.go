package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestApproveWithoutArtifactFails(t *testing.T) {
	eng := engine.New(state.NewRunDir(t.TempDir(), "missing"), t.TempDir())

	err := eng.Approve(context.Background())
	if err == nil {
		t.Fatal("Approve() error = nil, want read artifact error")
	}
	if !strings.Contains(err.Error(), "read artifact") {
		t.Fatalf("Approve() error = %q, want read artifact context", err)
	}
}

func TestVerifyTaskWritesVerifierResult(t *testing.T) {
	dir := t.TempDir()
	runDir := state.NewRunDir(dir, "run-verify")
	if err := runDir.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := runDir.WriteArtifact(&state.RunArtifact{
		SchemaVersion: "0.1",
		RunID:         "run-verify",
	}); err != nil {
		t.Fatalf("WriteArtifact: %v", err)
	}

	task := &state.Task{
		TaskID:           "task-verify",
		RequirementIDs:   []string{"AT-FR-001"},
		RequiredEvidence: []string{"quality_gate_pass"},
	}
	report := &state.CompletionReport{
		TaskID:    task.TaskID,
		AttemptID: "attempt-verify",
	}

	eng := engine.New(runDir, dir)
	result, err := eng.VerifyTask(context.Background(), task, report)
	if err != nil {
		t.Fatalf("VerifyTask: %v", err)
	}
	if !result.Pass {
		t.Fatalf("VerifyTask pass = false, findings = %+v", result.BlockingFindings)
	}

	var persisted state.VerifierResult
	resultPath := filepath.Join(runDir.ReportDir(task.TaskID), "verifier-result.json")
	if err := state.ReadJSON(resultPath, &persisted); err != nil {
		t.Fatalf("ReadJSON(verifier-result): %v", err)
	}
	if persisted.TaskID != task.TaskID || persisted.AttemptID != report.AttemptID {
		t.Fatalf("persisted verifier result = %+v, want task %q attempt %q", persisted, task.TaskID, report.AttemptID)
	}
}

func TestUpdateTaskStatusReturnsErrorWhenTaskMissing(t *testing.T) {
	dir := t.TempDir()
	runDir := state.NewRunDir(dir, "run-update-missing")
	if err := runDir.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := runDir.WriteTasks([]state.Task{{TaskID: "task-1"}}); err != nil {
		t.Fatalf("WriteTasks: %v", err)
	}

	eng := engine.New(runDir, dir)
	err := eng.UpdateTaskStatus("task-missing", state.TaskDone, "verified")
	if err == nil {
		t.Fatal("UpdateTaskStatus() error = nil, want not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("UpdateTaskStatus() error = %q, want not found", err)
	}
}

func TestGetPendingTasksRespectsDependencies(t *testing.T) {
	dir := t.TempDir()
	runDir := state.NewRunDir(dir, "run-pending")
	if err := runDir.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := runDir.WriteTasks([]state.Task{
		{TaskID: "task-ready", Status: state.TaskPending},
		{TaskID: "task-done", Status: state.TaskDone},
		{TaskID: "task-blocked", Status: state.TaskPending, DependsOn: []string{"task-done", "task-missing"}},
		{TaskID: "task-waiting", Status: state.TaskPending, DependsOn: []string{"task-done"}},
		{TaskID: "task-failed", Status: state.TaskFailed},
	}); err != nil {
		t.Fatalf("WriteTasks: %v", err)
	}

	eng := engine.New(runDir, dir)
	pending, err := eng.GetPendingTasks()
	if err != nil {
		t.Fatalf("GetPendingTasks: %v", err)
	}

	if len(pending) != 2 {
		t.Fatalf("len(pending) = %d, want 2", len(pending))
	}
	if pending[0].TaskID != "task-ready" || pending[1].TaskID != "task-waiting" {
		t.Fatalf("pending tasks = %+v, want ready/waiting only", pending)
	}
}
