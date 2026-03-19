package ticket

import (
	"strings"
	"testing"
	"time"

	"github.com/runger/attest/internal/state"
)

func testTask() state.Task {
	now := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	return state.Task{
		TaskID:         "task-slice-1",
		Slug:           "task_slice_1",
		Title:          "Implement auth module",
		TaskType:       "implementation",
		Tags:           []string{"auth", "high-risk"},
		CreatedAt:      now,
		UpdatedAt:      now,
		Order:          2,
		ETag:           "a1b2c3d4",
		LineageID:      "task-slice-1",
		RequirementIDs: []string{"REQ-AUTH-1"},
		DependsOn:      []string{"task-slice-0"},
		Scope: state.TaskScope{
			OwnedPaths:    []string{"internal/auth"},
			IsolationMode: "direct",
		},
		Priority:         1,
		RiskLevel:        "high",
		DefaultModel:     "sonnet",
		Status:           state.TaskPending,
		RequiredEvidence: []string{"quality_gate_pass"},
		ParentTaskID:     "run-123",
	}
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	task := testTask()

	data, err := MarshalTicket(&task)
	if err != nil {
		t.Fatalf("MarshalTicket: %v", err)
	}

	got, err := UnmarshalTicket(data)
	if err != nil {
		t.Fatalf("UnmarshalTicket: %v", err)
	}

	if got.TaskID != task.TaskID {
		t.Errorf("TaskID = %q, want %q", got.TaskID, task.TaskID)
	}
	if got.Title != task.Title {
		t.Errorf("Title = %q, want %q", got.Title, task.Title)
	}
	if got.Status != task.Status {
		t.Errorf("Status = %q, want %q", got.Status, task.Status)
	}
	if got.Priority != task.Priority {
		t.Errorf("Priority = %d, want %d", got.Priority, task.Priority)
	}
	if got.Order != task.Order {
		t.Errorf("Order = %d, want %d", got.Order, task.Order)
	}
	if got.ParentTaskID != task.ParentTaskID {
		t.Errorf("ParentTaskID = %q, want %q", got.ParentTaskID, task.ParentTaskID)
	}
	if len(got.DependsOn) != len(task.DependsOn) {
		t.Errorf("DependsOn len = %d, want %d", len(got.DependsOn), len(task.DependsOn))
	}
	if len(got.Scope.OwnedPaths) != len(task.Scope.OwnedPaths) {
		t.Errorf("OwnedPaths len = %d, want %d", len(got.Scope.OwnedPaths), len(task.Scope.OwnedPaths))
	}
}

func TestStatusMapping(t *testing.T) {
	tests := []struct {
		attest state.TaskStatus
		wantTk string
	}{
		{state.TaskPending, "open"},
		{state.TaskClaimed, "in_progress"},
		{state.TaskImplementing, "in_progress"},
		{state.TaskVerifying, "in_progress"},
		{state.TaskUnderReview, "in_progress"},
		{state.TaskRepairPending, "open"},
		{state.TaskBlocked, "open"},
		{state.TaskDone, "closed"},
		{state.TaskFailed, "closed"},
	}
	for _, tt := range tests {
		got := StatusToTicket(tt.attest)
		if got != tt.wantTk {
			t.Errorf("StatusToTicket(%s) = %q, want %q", tt.attest, got, tt.wantTk)
		}
	}
}

func TestStatusFromTicketPrefersAttestStatus(t *testing.T) {
	got := StatusFromTicket("open", "implementing")
	if got != state.TaskImplementing {
		t.Errorf("StatusFromTicket(open, implementing) = %q, want implementing", got)
	}
}

func TestStatusFromTicketFallsBackToTkStatus(t *testing.T) {
	got := StatusFromTicket("in_progress", "")
	if got != state.TaskClaimed {
		t.Errorf("StatusFromTicket(in_progress, '') = %q, want claimed", got)
	}
}

func TestMarshalContainsFrontmatterDelimiters(t *testing.T) {
	task := testTask()
	data, err := MarshalTicket(&task)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		t.Error("missing opening frontmatter delimiter")
	}
	if !strings.Contains(content, "\n---\n") {
		t.Error("missing closing frontmatter delimiter")
	}
	if !strings.Contains(content, "# Implement auth module") {
		t.Error("missing title heading")
	}
}

func TestMarshalContainsAttestFields(t *testing.T) {
	task := testTask()
	data, _ := MarshalTicket(&task)
	content := string(data)

	for _, field := range []string{"attest_status:", "requirement_ids:", "risk_level:", "owned_paths:"} {
		if !strings.Contains(content, field) {
			t.Errorf("missing field %q in output", field)
		}
	}
}

func TestOrderZeroIsPreserved(t *testing.T) {
	task := testTask()
	task.Order = 0

	data, err := MarshalTicket(&task)
	if err != nil {
		t.Fatal(err)
	}

	got, err := UnmarshalTicket(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Order != 0 {
		t.Errorf("Order = %d, want 0 (must not be dropped by omitempty)", got.Order)
	}
}

func TestMalformedYAML(t *testing.T) {
	_, err := UnmarshalTicket([]byte("not yaml at all"))
	if err == nil {
		t.Fatal("should error on malformed input")
	}
}

func TestUpdateFrontmatterPreservesBody(t *testing.T) {
	original := []byte("---\nid: test\nstatus: open\npriority: 0\norder: 0\n---\n# Test\n\nSome description.\n\n## Notes\n\n**2026-03-19T10:00:00Z**\n\nImportant note.\n")

	task := &state.Task{
		TaskID: "test",
		Title:  "Test",
		Status: state.TaskDone,
	}

	updated, err := UpdateFrontmatter(original, task)
	if err != nil {
		t.Fatalf("UpdateFrontmatter: %v", err)
	}

	content := string(updated)
	if !strings.Contains(content, "Important note.") {
		t.Error("body content was lost")
	}
	if !strings.Contains(content, "## Notes") {
		t.Error("Notes section was lost")
	}
	if !strings.Contains(content, "attest_status: done") {
		t.Error("attest_status not updated")
	}
}
