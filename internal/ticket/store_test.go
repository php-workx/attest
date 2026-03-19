package ticket

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/runger/attest/internal/state"
)

func TestCreateAndRead(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	task := testTask()
	if err := store.WriteTask(&task); err != nil {
		t.Fatalf("WriteTask: %v", err)
	}

	got, err := store.ReadTask("task-slice-1")
	if err != nil {
		t.Fatalf("ReadTask: %v", err)
	}
	if got.TaskID != task.TaskID {
		t.Errorf("TaskID = %q, want %q", got.TaskID, task.TaskID)
	}
	if got.Title != task.Title {
		t.Errorf("Title = %q, want %q", got.Title, task.Title)
	}
}

func TestPartialIDMatch(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	const testID = "abc-1234"
	task := testTask()
	task.TaskID = testID
	if err := store.WriteTask(&task); err != nil {
		t.Fatal(err)
	}

	// Full match.
	got, err := store.ReadTask(testID)
	if err != nil {
		t.Fatalf("full match: %v", err)
	}
	if got.TaskID != testID {
		t.Errorf("full match ID = %q", got.TaskID)
	}

	// Partial match.
	got, err = store.ReadTask("1234")
	if err != nil {
		t.Fatalf("partial match: %v", err)
	}
	if got.TaskID != testID {
		t.Errorf("partial match ID = %q", got.TaskID)
	}
}

func TestAmbiguousID(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	for _, id := range []string{"abc-1234", "abc-1235"} {
		task := testTask()
		task.TaskID = id
		if err := store.WriteTask(&task); err != nil {
			t.Fatal(err)
		}
	}

	_, err := store.ReadTask("abc-123")
	if err == nil {
		t.Fatal("should return ambiguous error")
	}
}

func TestNotFoundID(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	_, err := store.ReadTask("nonexistent")
	if err == nil {
		t.Fatal("should return not found error")
	}
}

func TestUpdateStatus(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	task := testTask()
	if err := store.WriteTask(&task); err != nil {
		t.Fatal(err)
	}

	if err := store.UpdateStatus("task-slice-1", state.TaskDone, "verified"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	got, err := store.ReadTask("task-slice-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.TaskDone {
		t.Errorf("Status = %q, want done", got.Status)
	}
	if got.StatusReason != "verified" {
		t.Errorf("StatusReason = %q, want verified", got.StatusReason)
	}
}

func TestWriteTasksCreatesDirectoryAndEpic(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "subdir", ".tickets")
	store := NewStore(dir)

	tasks := []state.Task{testTask()}
	if err := store.WriteTasks("run-123", tasks); err != nil {
		t.Fatalf("WriteTasks: %v", err)
	}

	// Epic should exist.
	epicPath := filepath.Join(dir, "run-123.md")
	if _, err := os.Stat(epicPath); err != nil {
		t.Errorf("epic file missing: %v", err)
	}

	// Task should exist with parent.
	got, err := store.ReadTask("task-slice-1")
	if err != nil {
		t.Fatalf("ReadTask: %v", err)
	}
	if got.ParentTaskID != "run-123" {
		t.Errorf("ParentTaskID = %q, want run-123", got.ParentTaskID)
	}
}

func TestReadTasksScopedToRun(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// Write tasks for two runs.
	task1 := testTask()
	task1.TaskID = "task-a"
	task1.ParentTaskID = "run-1"
	task2 := testTask()
	task2.TaskID = "task-b"
	task2.ParentTaskID = "run-2"

	if err := store.WriteTask(&task1); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteTask(&task2); err != nil {
		t.Fatal(err)
	}

	// ReadTasks for run-1 should only return task-a.
	tasks, err := store.ReadTasks("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != "task-a" {
		t.Errorf("ReadTasks(run-1) = %v, want [task-a]", tasks)
	}
}

func TestPartialIDMatchPrefix(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	const testID = "abc-1234"
	task := testTask()
	task.TaskID = testID
	if err := store.WriteTask(&task); err != nil {
		t.Fatal(err)
	}

	// Match by prefix "abc-1".
	got, err := store.ReadTask("abc-1")
	if err != nil {
		t.Fatalf("prefix match: %v", err)
	}
	if got.TaskID != testID {
		t.Errorf("prefix match ID = %q, want %q", got.TaskID, testID)
	}
}

func TestConcurrentWriteSameTicket(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// Create initial ticket.
	task := testTask()
	task.TaskID = "shared"
	if err := store.WriteTask(&task); err != nil {
		t.Fatal(err)
	}

	// 10 goroutines updating the same ticket concurrently.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = store.UpdateStatus("shared", state.TaskPending, fmt.Sprintf("update-%d", n))
		}(i)
	}
	wg.Wait()

	// Ticket should still be readable and valid.
	got, err := store.ReadTask("shared")
	if err != nil {
		t.Fatalf("ReadTask after concurrent writes: %v", err)
	}
	if got.TaskID != "shared" {
		t.Errorf("TaskID = %q, want shared", got.TaskID)
	}
}

func TestConcurrentWritesDifferentTickets(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			task := testTask()
			task.TaskID = fmt.Sprintf("task-%d", n)
			_ = store.WriteTask(&task)
		}(i)
	}
	wg.Wait()

	// All 10 should exist.
	tasks, err := store.readAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 10 {
		t.Errorf("got %d tasks, want 10", len(tasks))
	}
}
