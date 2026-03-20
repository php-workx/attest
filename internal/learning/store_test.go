package learning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestAddAndGet(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	l := &Learning{
		Tags:     []string{"compiler", "grouping"},
		Category: CategoryPattern,
		Content:  "Group requirements by source line proximity",
		Summary:  "Proximity grouping reduces task count",
	}
	if err := store.Add(l); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if l.ID == "" {
		t.Error("ID not generated")
	}

	got, err := store.Get(l.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Summary != l.Summary {
		t.Errorf("Summary = %q, want %q", got.Summary, l.Summary)
	}
	if got.Confidence != 0.5 {
		t.Errorf("Confidence = %f, want 0.5", got.Confidence)
	}
}

func TestQueryByTag(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	_ = store.Add(&Learning{Tags: []string{"compiler"}, Category: CategoryPattern, Summary: "A"})
	_ = store.Add(&Learning{Tags: []string{"ticket"}, Category: CategoryAntiPattern, Summary: "B"})
	_ = store.Add(&Learning{Tags: []string{"compiler", "flock"}, Category: CategoryTooling, Summary: "C"})

	results, err := store.Query(QueryOpts{Tags: []string{"compiler"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestQueryByCategory(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	_ = store.Add(&Learning{Tags: []string{"a"}, Category: CategoryPattern, Summary: "pat"})
	_ = store.Add(&Learning{Tags: []string{"b"}, Category: CategoryAntiPattern, Summary: "anti"})

	results, err := store.Query(QueryOpts{Category: CategoryAntiPattern})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Summary != "anti" {
		t.Fatalf("got %v, want [anti]", results)
	}
}

func TestQueryByPaths(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	_ = store.Add(&Learning{Tags: []string{"a"}, Category: CategoryCodebase, Summary: "engine", SourcePaths: []string{"internal/engine"}})
	_ = store.Add(&Learning{Tags: []string{"b"}, Category: CategoryCodebase, Summary: "ticket", SourcePaths: []string{"internal/ticket"}})

	results, err := store.Query(QueryOpts{Paths: []string{"internal/engine"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Summary != "engine" {
		t.Fatalf("got %v, want [engine]", results)
	}
}

func TestQuerySortByUtility(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	_ = store.Add(&Learning{Tags: []string{"a"}, Category: CategoryPattern, Summary: "low", Utility: 0.3})
	_ = store.Add(&Learning{Tags: []string{"a"}, Category: CategoryPattern, Summary: "high", Utility: 0.9})
	_ = store.Add(&Learning{Tags: []string{"a"}, Category: CategoryPattern, Summary: "mid", Utility: 0.6})

	results, err := store.Query(QueryOpts{Tags: []string{"a"}, SortBy: "utility"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d, want 3", len(results))
	}
	if results[0].Summary != "high" || results[1].Summary != "mid" || results[2].Summary != "low" {
		t.Errorf("wrong order: %s, %s, %s", results[0].Summary, results[1].Summary, results[2].Summary)
	}
}

func TestQueryLimit(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	for i := 0; i < 10; i++ {
		_ = store.Add(&Learning{Tags: []string{"bulk"}, Category: CategoryPattern, Summary: "item"})
	}

	results, err := store.Query(QueryOpts{Tags: []string{"bulk"}, Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d, want 3", len(results))
	}
}

func TestQueryMinUtility(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	_ = store.Add(&Learning{Tags: []string{"a"}, Category: CategoryPattern, Summary: "low", Utility: 0.2})
	_ = store.Add(&Learning{Tags: []string{"a"}, Category: CategoryPattern, Summary: "high", Utility: 0.8})

	results, err := store.Query(QueryOpts{Tags: []string{"a"}, MinUtility: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Summary != "high" {
		t.Fatalf("got %v, want [high]", results)
	}
}

func TestRecordCitation(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	l := &Learning{Tags: []string{"a"}, Category: CategoryPattern, Summary: "test"}
	_ = store.Add(l)

	if err := store.RecordCitation(l.ID); err != nil {
		t.Fatalf("RecordCitation: %v", err)
	}

	got, _ := store.Get(l.ID)
	if got.CitedCount != 1 {
		t.Errorf("CitedCount = %d, want 1", got.CitedCount)
	}
	if got.LastCitedAt == nil {
		t.Error("LastCitedAt should be set")
	}
}

func TestUtilityDecay(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	store := &Store{Dir: dir, Now: fixedClock(now)}

	// Add a learning created 60 days ago, never cited.
	old := &Learning{
		Tags:      []string{"stale"},
		Category:  CategoryPattern,
		Summary:   "old learning",
		Utility:   0.5,
		CreatedAt: now.Add(-60 * 24 * time.Hour),
	}
	_ = store.Add(old)

	results, err := store.Query(QueryOpts{Tags: []string{"stale"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
	// Utility should have decayed by 0.05.
	if results[0].Utility != 0.45 {
		t.Errorf("Utility = %f, want 0.45 (decayed)", results[0].Utility)
	}
}

func TestExpiredAndSupersededExcluded(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	_ = store.Add(&Learning{Tags: []string{"a"}, Category: CategoryPattern, Summary: "active"})
	_ = store.Add(&Learning{Tags: []string{"a"}, Category: CategoryPattern, Summary: "expired", Expired: true})
	_ = store.Add(&Learning{Tags: []string{"a"}, Category: CategoryPattern, Summary: "superseded", SupersededBy: "lrn-other"})

	results, err := store.Query(QueryOpts{Tags: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Summary != "active" {
		t.Fatalf("got %v, want [active]", results)
	}
}

func TestHandoffWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	h := &SessionHandoff{
		RunID:     "run-1234",
		TaskID:    "task-a",
		Summary:   "Implemented grouping, tests passing",
		NextSteps: []string{"Wire into engine", "Add CLI"},
	}
	if err := store.WriteHandoff(h); err != nil {
		t.Fatalf("WriteHandoff: %v", err)
	}

	got, err := store.LatestHandoff()
	if err != nil {
		t.Fatalf("LatestHandoff: %v", err)
	}
	if got == nil {
		t.Fatal("no handoff returned")
	}
	if got.Summary != h.Summary {
		t.Errorf("Summary = %q, want %q", got.Summary, h.Summary)
	}
	if len(got.NextSteps) != 2 {
		t.Errorf("NextSteps len = %d, want 2", len(got.NextSteps))
	}
}

func TestLatestHandoffNone(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	got, err := store.LatestHandoff()
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil handoff when none exists")
	}
}

func TestGarbageCollect(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	store := &Store{Dir: dir, Now: fixedClock(now)}

	// Active learning.
	_ = store.Add(&Learning{Tags: []string{"a"}, Category: CategoryPattern, Summary: "keep"})

	// Expired learning created 100 days ago.
	expired := &Learning{
		Tags:      []string{"a"},
		Category:  CategoryPattern,
		Summary:   "remove",
		Expired:   true,
		CreatedAt: now.Add(-100 * 24 * time.Hour),
	}
	_ = store.Add(expired)

	removed, err := store.GarbageCollect(90 * 24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}

	// Verify only active learning remains.
	results, _ := store.Query(QueryOpts{Tags: []string{"a"}})
	if len(results) != 1 || results[0].Summary != "keep" {
		t.Errorf("after GC: %v, want [keep]", results)
	}
}

func TestTagIndexRebuilt(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	_ = store.Add(&Learning{Tags: []string{"alpha", "beta"}, Category: CategoryPattern, Summary: "test"})

	// Verify tags.json exists and has content.
	data, err := os.ReadFile(filepath.Join(dir, "tags.json"))
	if err != nil {
		t.Fatalf("read tags.json: %v", err)
	}
	content := string(data)
	if !contains(content, "alpha") || !contains(content, "beta") {
		t.Errorf("tags.json missing expected tags: %s", content)
	}
}

func TestGetNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	_, err := store.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent ID")
	}
}

func TestTagNormalization(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	_ = store.Add(&Learning{Tags: []string{"  UPPER  ", "Mixed"}, Category: CategoryPattern, Summary: "test"})

	// Query with lowercase should match.
	results, err := store.Query(QueryOpts{Tags: []string{"upper"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("got %d, want 1 (case-insensitive tag match)", len(results))
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
