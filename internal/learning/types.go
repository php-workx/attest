// Package learning provides a project-scoped learning store for cross-session
// knowledge persistence. Learnings are tagged entries stored in JSONL format
// with utility scoring and lazy decay.
package learning

import "time"

// Category classifies the kind of learning.
type Category string

// Learning categories.
const (
	CategoryPattern     Category = "pattern"      // Reusable approach that worked
	CategoryAntiPattern Category = "anti_pattern" // Approach that failed
	CategoryTooling     Category = "tooling"      // Build/lint/test insight
	CategoryCodebase    Category = "codebase"     // Project-specific knowledge
	CategoryProcess     Category = "process"      // Workflow/dispatch insight
)

// Learning is a single entry in the learning store.
type Learning struct {
	ID           string     `json:"id"`
	CreatedAt    time.Time  `json:"created_at"`
	Tags         []string   `json:"tags"`
	Category     Category   `json:"category"`
	Content      string     `json:"content"`
	Summary      string     `json:"summary"`
	SourceTask   string     `json:"source_task,omitempty"`
	SourceRun    string     `json:"source_run,omitempty"`
	SourcePaths  []string   `json:"source_paths,omitempty"`
	Confidence   float64    `json:"confidence"`
	Utility      float64    `json:"utility"`
	CitedCount   int        `json:"cited_count"`
	LastCitedAt  *time.Time `json:"last_cited_at,omitempty"`
	SupersededBy string     `json:"superseded_by,omitempty"`
	Expired      bool       `json:"expired"`
}

// SessionHandoff captures session continuity state.
type SessionHandoff struct {
	ID            string    `json:"id"`
	CreatedAt     time.Time `json:"created_at"`
	RunID         string    `json:"run_id,omitempty"`
	TaskID        string    `json:"task_id,omitempty"`
	Summary       string    `json:"summary"`
	NextSteps     []string  `json:"next_steps,omitempty"`
	OpenQuestions []string  `json:"open_questions,omitempty"`
	LearningIDs   []string  `json:"learning_ids,omitempty"`
}

// QueryOpts controls learning retrieval filtering.
type QueryOpts struct {
	Tags       []string // match any tag
	Category   Category // exact category match (empty = any)
	Paths      []string // match learnings with overlapping SourcePaths
	MinUtility float64  // utility threshold (default 0.0)
	Limit      int      // max results (0 = unlimited)
	SortBy     string   // "utility" (default), "created_at"
}

// TagIndex is the inverted tag index for fast lookups.
type TagIndex struct {
	Tags    map[string][]string `json:"tags"`
	Version int                 `json:"version"`
}
