package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	status, err := eng.RunDir.ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus after Prepare: %v", err)
	}
	if status.State != state.RunAwaitingApproval {
		t.Fatalf("status state after Prepare = %s, want %s", status.State, state.RunAwaitingApproval)
	}

	// Approve.
	if err := eng.Approve(ctx); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	status, err = eng.RunDir.ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus after Approve: %v", err)
	}
	if status.State != state.RunApproved {
		t.Fatalf("status state after Approve = %s, want %s", status.State, state.RunApproved)
	}

	// Compile.
	result, err := eng.Compile(ctx)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(result.Tasks) == 0 {
		t.Fatal("got 0 tasks, want at least 1")
	}
	status, err = eng.RunDir.ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus after Compile: %v", err)
	}
	if status.State != state.RunRunning {
		t.Fatalf("status state after Compile = %s, want %s", status.State, state.RunRunning)
	}
	if status.TaskCountsByState[string(state.TaskPending)] != len(result.Tasks) {
		t.Fatalf("pending count after Compile = %d, want %d", status.TaskCountsByState[string(state.TaskPending)], len(result.Tasks))
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
	if len(pending) != 2 {
		t.Fatalf("got %d pending tasks, want 2 grouped tasks", len(pending))
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

	status, err := runDir.ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus after VerifyTask: %v", err)
	}
	if status.State != state.RunCompleted {
		t.Fatalf("status state after successful VerifyTask = %s, want %s", status.State, state.RunCompleted)
	}

	tasks, err := runDir.ReadTasks()
	if err != nil {
		t.Fatalf("ReadTasks after VerifyTask: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Status != state.TaskDone {
		t.Fatalf("task status after successful VerifyTask = %+v, want done", tasks)
	}
}

func TestVerifyTaskSatisfiesCoverageAndCompletesRun(t *testing.T) {
	dir := t.TempDir()
	runDir := state.NewRunDir(dir, "run-complete")
	if err := runDir.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := runDir.WriteArtifact(&state.RunArtifact{
		SchemaVersion: "0.1",
		RunID:         "run-complete",
	}); err != nil {
		t.Fatalf("WriteArtifact: %v", err)
	}
	if err := runDir.WriteTasks([]state.Task{{
		TaskID:           "task-complete",
		Status:           state.TaskPending,
		RequirementIDs:   []string{"AT-FR-001"},
		RequiredEvidence: []string{"quality_gate_pass"},
	}}); err != nil {
		t.Fatalf("WriteTasks: %v", err)
	}
	if err := runDir.WriteCoverage([]state.RequirementCoverage{{
		RequirementID:   "AT-FR-001",
		Status:          "in_progress",
		CoveringTaskIDs: []string{"task-complete"},
	}}); err != nil {
		t.Fatalf("WriteCoverage: %v", err)
	}
	if err := runDir.WriteStatus(&state.RunStatus{
		RunID:              "run-complete",
		State:              state.RunRunning,
		LastTransitionTime: time.Now(),
		TaskCountsByState:  map[string]int{"pending": 1},
	}); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	task := &state.Task{
		TaskID:           "task-complete",
		RequirementIDs:   []string{"AT-FR-001"},
		RequiredEvidence: []string{"quality_gate_pass"},
	}
	report := &state.CompletionReport{
		TaskID:    task.TaskID,
		AttemptID: "attempt-complete",
	}

	eng := engine.New(runDir, dir)
	result, err := eng.VerifyTask(context.Background(), task, report)
	if err != nil {
		t.Fatalf("VerifyTask: %v", err)
	}
	if !result.Pass {
		t.Fatalf("VerifyTask pass = false, findings = %+v", result.BlockingFindings)
	}

	coverage, err := runDir.ReadCoverage()
	if err != nil {
		t.Fatalf("ReadCoverage after VerifyTask: %v", err)
	}
	if len(coverage) != 1 || coverage[0].Status != "satisfied" {
		t.Fatalf("coverage after successful VerifyTask = %+v, want satisfied", coverage)
	}

	status, err := runDir.ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus after VerifyTask: %v", err)
	}
	if status.State != state.RunCompleted {
		t.Fatalf("status state after successful VerifyTask = %s, want %s", status.State, state.RunCompleted)
	}
	if status.CurrentGate != "completed" {
		t.Fatalf("current gate after successful VerifyTask = %q, want completed", status.CurrentGate)
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

func TestVerifyTaskFailureBlocksRun(t *testing.T) {
	dir := t.TempDir()
	runDir := state.NewRunDir(dir, "run-blocked")
	if err := runDir.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := runDir.WriteArtifact(&state.RunArtifact{
		SchemaVersion: "0.1",
		RunID:         "run-blocked",
	}); err != nil {
		t.Fatalf("WriteArtifact: %v", err)
	}
	if err := runDir.WriteTasks([]state.Task{{
		TaskID:           "task-blocked",
		Status:           state.TaskPending,
		RequirementIDs:   []string{"AT-FR-001"},
		RequiredEvidence: []string{"file_exists"},
	}}); err != nil {
		t.Fatalf("WriteTasks: %v", err)
	}
	if err := runDir.WriteCoverage([]state.RequirementCoverage{{
		RequirementID:   "AT-FR-001",
		Status:          "in_progress",
		CoveringTaskIDs: []string{"task-blocked"},
	}}); err != nil {
		t.Fatalf("WriteCoverage: %v", err)
	}
	if err := runDir.WriteStatus(&state.RunStatus{
		RunID:              "run-blocked",
		State:              state.RunRunning,
		LastTransitionTime: time.Now(),
		TaskCountsByState:  map[string]int{},
	}); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	task := &state.Task{
		TaskID:           "task-blocked",
		RequirementIDs:   []string{"AT-FR-001"},
		RequiredEvidence: []string{"file_exists"},
	}
	report := &state.CompletionReport{
		TaskID:    task.TaskID,
		AttemptID: "attempt-blocked",
	}

	eng := engine.New(runDir, dir)
	result, err := eng.VerifyTask(context.Background(), task, report)
	if err != nil {
		t.Fatalf("VerifyTask: %v", err)
	}
	if result.Pass {
		t.Fatal("VerifyTask unexpectedly passed")
	}

	status, err := runDir.ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus after failed VerifyTask: %v", err)
	}
	if status.State != state.RunBlocked {
		t.Fatalf("status state after failed VerifyTask = %s, want %s", status.State, state.RunBlocked)
	}
	if len(status.OpenBlockers) == 0 {
		t.Fatal("OpenBlockers after failed VerifyTask = empty, want blocker detail")
	}

	tasks, err := runDir.ReadTasks()
	if err != nil {
		t.Fatalf("ReadTasks after failed VerifyTask: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Status != state.TaskBlocked {
		t.Fatalf("task status after failed VerifyTask = %+v, want blocked", tasks)
	}

	coverage, err := runDir.ReadCoverage()
	if err != nil {
		t.Fatalf("ReadCoverage after failed VerifyTask: %v", err)
	}
	if len(coverage) != 1 || coverage[0].Status != "blocked" {
		t.Fatalf("coverage after failed VerifyTask = %+v, want blocked", coverage)
	}
}

func TestRetryTaskRequeuesBlockedTaskAndRestoresCoverage(t *testing.T) {
	dir := t.TempDir()
	runDir := state.NewRunDir(dir, "run-retry")
	if err := runDir.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := runDir.WriteTasks([]state.Task{{
		TaskID:         "task-retry",
		Status:         state.TaskBlocked,
		StatusReason:   "missing evidence",
		RequirementIDs: []string{"AT-FR-001"},
	}}); err != nil {
		t.Fatalf("WriteTasks: %v", err)
	}
	if err := runDir.WriteCoverage([]state.RequirementCoverage{{
		RequirementID:   "AT-FR-001",
		Status:          "blocked",
		CoveringTaskIDs: []string{"task-retry"},
	}}); err != nil {
		t.Fatalf("WriteCoverage: %v", err)
	}
	if err := runDir.WriteStatus(&state.RunStatus{
		RunID:              "run-retry",
		State:              state.RunBlocked,
		LastTransitionTime: time.Now(),
		TaskCountsByState:  map[string]int{"blocked": 1},
		OpenBlockers:       []string{"task-retry: missing evidence"},
	}); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	eng := engine.New(runDir, dir)
	if err := eng.RetryTask("task-retry"); err != nil {
		t.Fatalf("RetryTask: %v", err)
	}

	tasks, err := runDir.ReadTasks()
	if err != nil {
		t.Fatalf("ReadTasks after RetryTask: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Status != state.TaskPending || tasks[0].StatusReason != "" {
		t.Fatalf("task after RetryTask = %+v, want pending with cleared reason", tasks)
	}

	coverage, err := runDir.ReadCoverage()
	if err != nil {
		t.Fatalf("ReadCoverage after RetryTask: %v", err)
	}
	if len(coverage) != 1 || coverage[0].Status != "in_progress" {
		t.Fatalf("coverage after RetryTask = %+v, want in_progress", coverage)
	}

	status, err := runDir.ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus after RetryTask: %v", err)
	}
	if status.State != state.RunRunning {
		t.Fatalf("status state after RetryTask = %s, want %s", status.State, state.RunRunning)
	}
	if len(status.OpenBlockers) != 0 {
		t.Fatalf("OpenBlockers after RetryTask = %+v, want empty", status.OpenBlockers)
	}
}
