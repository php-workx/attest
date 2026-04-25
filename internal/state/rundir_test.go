package state_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/php-workx/fabrikk/internal/state"
)

func TestRunDirInitCreatesDirectoryStructure(t *testing.T) {
	baseDir := t.TempDir()
	runDir := state.NewRunDir(baseDir, "run-123")

	if err := runDir.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	for _, path := range []string{
		runDir.Root,
		filepath.Join(runDir.Root, "claims"),
		filepath.Join(runDir.Root, "reports"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%s): %v", path, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", path)
		}
	}
}

func TestRunDirPathHelpers(t *testing.T) {
	runDir := state.NewRunDir("/tmp/project", "run-456")

	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "artifact", got: runDir.Artifact(), want: "/tmp/project/.fabrikk/runs/run-456/run-artifact.json"},
		{name: "tasks", got: runDir.Tasks(), want: "/tmp/project/.fabrikk/runs/run-456/tasks.json"},
		{name: "coverage", got: runDir.Coverage(), want: "/tmp/project/.fabrikk/runs/run-456/requirement-coverage.json"},
		{name: "status", got: runDir.Status(), want: "/tmp/project/.fabrikk/runs/run-456/run-status.json"},
		{name: "events", got: runDir.Events(), want: "/tmp/project/.fabrikk/runs/run-456/events.jsonl"},
		{name: "clarifications", got: runDir.Clarifications(), want: "/tmp/project/.fabrikk/runs/run-456/clarifications.json"},
		{name: "engine", got: runDir.Engine(), want: "/tmp/project/.fabrikk/runs/run-456/engine.json"},
		{name: "engine log", got: runDir.EngineLog(), want: "/tmp/project/.fabrikk/runs/run-456/engine.log"},
		{name: "technical spec", got: runDir.TechnicalSpec(), want: "/tmp/project/.fabrikk/runs/run-456/technical-spec.md"},
		{name: "technical spec review", got: runDir.TechnicalSpecReview(), want: "/tmp/project/.fabrikk/runs/run-456/technical-spec-review.json"},
		{name: "technical spec approval", got: runDir.TechnicalSpecApproval(), want: "/tmp/project/.fabrikk/runs/run-456/technical-spec-approval.json"},
		{name: "execution plan", got: runDir.ExecutionPlan(), want: "/tmp/project/.fabrikk/runs/run-456/execution-plan.json"},
		{name: "execution plan markdown", got: runDir.ExecutionPlanMarkdown(), want: "/tmp/project/.fabrikk/runs/run-456/execution-plan.md"},
		{name: "execution plan review", got: runDir.ExecutionPlanReview(), want: "/tmp/project/.fabrikk/runs/run-456/execution-plan-review.json"},
		{name: "execution plan approval", got: runDir.ExecutionPlanApproval(), want: "/tmp/project/.fabrikk/runs/run-456/execution-plan-approval.json"},
		{name: "claim path", got: runDir.ClaimPath("task-1"), want: "/tmp/project/.fabrikk/runs/run-456/claims/task-1.json"},
		{name: "report dir", got: runDir.ReportDir("task-1"), want: "/tmp/project/.fabrikk/runs/run-456/reports/task-1"},
		{name: "attempt path", got: runDir.AttemptPath("task-1"), want: "/tmp/project/.fabrikk/runs/run-456/reports/task-1/attempt.json"},
		{name: "council result path", got: runDir.CouncilResultPath("task-1"), want: "/tmp/project/.fabrikk/runs/run-456/reports/task-1/council-result.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestRunDirNormalizationPathHelpers(t *testing.T) {
	runDir := state.NewRunDir("/tmp/project", "run-456")

	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "normalized artifact candidate", got: runDir.NormalizedArtifactCandidate(), want: "/tmp/project/.fabrikk/runs/run-456/normalized-artifact-candidate.json"},
		{name: "source manifest", got: runDir.SpecNormalizationSourceManifest(), want: "/tmp/project/.fabrikk/runs/run-456/spec-normalization-source-manifest.json"},
		{name: "validation", got: runDir.SpecNormalizationValidation(), want: "/tmp/project/.fabrikk/runs/run-456/spec-normalization-validation.json"},
		{name: "review", got: runDir.SpecNormalizationReview(), want: "/tmp/project/.fabrikk/runs/run-456/spec-normalization-review.json"},
		{name: "approval", got: runDir.RunArtifactApproval(), want: "/tmp/project/.fabrikk/runs/run-456/run-artifact-approval.json"},
		{name: "converter prompt", got: runDir.SpecNormalizationConverterPrompt(), want: "/tmp/project/.fabrikk/runs/run-456/spec-normalization-converter-prompt.md"},
		{name: "converter raw", got: runDir.SpecNormalizationConverterRaw(), want: "/tmp/project/.fabrikk/runs/run-456/spec-normalization-converter-raw.txt"},
		{name: "verifier prompt", got: runDir.SpecNormalizationVerifierPrompt(), want: "/tmp/project/.fabrikk/runs/run-456/spec-normalization-verifier-prompt.md"},
		{name: "verifier raw", got: runDir.SpecNormalizationVerifierRaw(), want: "/tmp/project/.fabrikk/runs/run-456/spec-normalization-verifier-raw.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestRunDirNormalizationArtifactsRoundTrip(t *testing.T) {
	baseDir := t.TempDir()
	runDir := state.NewRunDir(baseDir, "run-normalized")
	if err := runDir.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	reviewedAt := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	approvedAt := reviewedAt.Add(30 * time.Minute)
	candidate := &state.RunArtifact{
		SchemaVersion: "0.1",
		RunID:         "run-normalized",
		Requirements: []state.Requirement{{
			ID:   "AT-FR-001",
			Text: "The system must ingest prose specs.",
		}},
	}
	manifest := &state.SpecNormalizationSourceManifest{
		SchemaVersion: "0.1",
		RunID:         "run-normalized",
		Sources: []state.SourceManifestEntry{{
			Path:        "docs/specs/freeform.md",
			Fingerprint: "sha256:source",
			ByteSize:    4096,
			LineCount:   87,
		}},
	}
	validation := []state.ReviewFinding{{
		FindingID: "snv-001",
		Severity:  "high",
		Category:  "missing_source_evidence",
		Summary:   "Requirement is missing source evidence.",
	}}
	review := &state.SpecNormalizationReview{
		SchemaVersion:          "0.1",
		RunID:                  "run-normalized",
		ArtifactType:           state.ArtifactTypeSpecNormalizationReview,
		Status:                 state.ReviewPass,
		Summary:                "Normalized artifact preserves source scope.",
		NormalizedArtifactHash: "sha256:artifact",
		SourceManifestHash:     "sha256:manifest",
		ReviewedInputHash:      "sha256:reviewed-input",
		ReviewedAt:             reviewedAt,
	}
	approval := &state.RunArtifactApproval{
		SchemaVersion:          "0.1",
		RunID:                  "run-normalized",
		ArtifactType:           state.ArtifactTypeRunArtifactApproval,
		NormalizedArtifactHash: "sha256:artifact",
		SourceManifestHash:     "sha256:manifest",
		ReviewedInputHash:      "sha256:reviewed-input",
		ApprovedAt:             approvedAt,
	}

	writes := []struct {
		name string
		run  func() error
	}{
		{name: "WriteNormalizedArtifactCandidate", run: func() error { return runDir.WriteNormalizedArtifactCandidate(candidate) }},
		{name: "WriteSpecNormalizationSourceManifest", run: func() error { return runDir.WriteSpecNormalizationSourceManifest(manifest) }},
		{name: "WriteSpecNormalizationValidation", run: func() error { return runDir.WriteSpecNormalizationValidation(validation) }},
		{name: "WriteSpecNormalizationReview", run: func() error { return runDir.WriteSpecNormalizationReview(review) }},
		{name: "WriteRunArtifactApproval", run: func() error { return runDir.WriteRunArtifactApproval(approval) }},
		{name: "WriteSpecNormalizationConverterPrompt", run: func() error { return runDir.WriteSpecNormalizationConverterPrompt([]byte("converter prompt")) }},
		{name: "WriteSpecNormalizationConverterRaw", run: func() error { return runDir.WriteSpecNormalizationConverterRaw([]byte("converter raw")) }},
		{name: "WriteSpecNormalizationVerifierPrompt", run: func() error { return runDir.WriteSpecNormalizationVerifierPrompt([]byte("verifier prompt")) }},
		{name: "WriteSpecNormalizationVerifierRaw", run: func() error { return runDir.WriteSpecNormalizationVerifierRaw([]byte("verifier raw")) }},
	}
	for _, write := range writes {
		if err := write.run(); err != nil {
			t.Fatalf("%s: %v", write.name, err)
		}
	}

	gotCandidate, err := runDir.ReadNormalizedArtifactCandidate()
	if err != nil {
		t.Fatalf("ReadNormalizedArtifactCandidate: %v", err)
	}
	if !reflect.DeepEqual(gotCandidate, candidate) {
		t.Fatalf("candidate = %+v, want %+v", gotCandidate, candidate)
	}
	gotManifest, err := runDir.ReadSpecNormalizationSourceManifest()
	if err != nil {
		t.Fatalf("ReadSpecNormalizationSourceManifest: %v", err)
	}
	if !reflect.DeepEqual(gotManifest, manifest) {
		t.Fatalf("manifest = %+v, want %+v", gotManifest, manifest)
	}
	gotValidation, err := runDir.ReadSpecNormalizationValidation()
	if err != nil {
		t.Fatalf("ReadSpecNormalizationValidation: %v", err)
	}
	if !reflect.DeepEqual(gotValidation, validation) {
		t.Fatalf("validation = %+v, want %+v", gotValidation, validation)
	}
	gotReview, err := runDir.ReadSpecNormalizationReview()
	if err != nil {
		t.Fatalf("ReadSpecNormalizationReview: %v", err)
	}
	if !reflect.DeepEqual(gotReview, review) {
		t.Fatalf("review = %+v, want %+v", gotReview, review)
	}
	gotApproval, err := runDir.ReadRunArtifactApproval()
	if err != nil {
		t.Fatalf("ReadRunArtifactApproval: %v", err)
	}
	if !reflect.DeepEqual(gotApproval, approval) {
		t.Fatalf("approval = %+v, want %+v", gotApproval, approval)
	}

	assertBytesFile(t, runDir.SpecNormalizationConverterPrompt(), []byte("converter prompt"))
	assertBytesFile(t, runDir.SpecNormalizationConverterRaw(), []byte("converter raw"))
	assertBytesFile(t, runDir.SpecNormalizationVerifierPrompt(), []byte("verifier prompt"))
	assertBytesFile(t, runDir.SpecNormalizationVerifierRaw(), []byte("verifier raw"))
}

func assertBytesFile(t *testing.T, path string, want []byte) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ReadFile(%s) = %q, want %q", path, got, want)
	}
}

type roundTripFixture struct {
	artifact         *state.RunArtifact
	tasks            []state.Task
	coverage         []state.RequirementCoverage
	status           *state.RunStatus
	techReview       *state.TechnicalSpecReview
	execPlan         *state.ExecutionPlan
	execReview       *state.ExecutionPlanReview
	approval         *state.ArtifactApproval
	techApproval     *state.ArtifactApproval
	techSpecMarkdown []byte
	execPlanMarkdown []byte
}

func newRoundTripFixture(now, approvedAt time.Time) roundTripFixture {
	return roundTripFixture{
		artifact: &state.RunArtifact{
			SchemaVersion: "0.1",
			RunID:         "run-roundtrip",
			SourceSpecs: []state.SourceSpec{
				{Path: "spec.md", Fingerprint: state.SHA256Bytes([]byte("spec"))},
			},
			Requirements: []state.Requirement{
				{ID: "AT-FR-001", Text: "The system must ingest specs."},
			},
			ApprovedAt:   &approvedAt,
			ApprovedBy:   "tester",
			ArtifactHash: state.SHA256Bytes([]byte("artifact")),
		},
		tasks: []state.Task{
			{
				TaskID:           "task-at-fr-001",
				Slug:             "task-at-fr-001",
				Title:            "Implement AT-FR-001",
				TaskType:         "implementation",
				CreatedAt:        now,
				UpdatedAt:        now,
				Order:            1,
				ETag:             state.SHA256Bytes([]byte("task")),
				LineageID:        "task-at-fr-001",
				RequirementIDs:   []string{"AT-FR-001"},
				Scope:            state.TaskScope{IsolationMode: "direct"},
				Priority:         1,
				RiskLevel:        "low",
				DefaultModel:     "sonnet",
				Status:           state.TaskPending,
				RequiredEvidence: []string{"quality_gate_pass"},
			},
		},
		coverage: []state.RequirementCoverage{
			{
				RequirementID:   "AT-FR-001",
				Status:          "in_progress",
				CoveringTaskIDs: []string{"task-at-fr-001"},
			},
		},
		status: &state.RunStatus{
			RunID:              "run-roundtrip",
			State:              state.RunApproved,
			LastTransitionTime: now,
			TaskCountsByState:  map[string]int{"pending": 1},
		},
		techReview: &state.TechnicalSpecReview{
			SchemaVersion:     "0.1",
			RunID:             "run-roundtrip",
			ArtifactType:      "technical_spec_review",
			TechnicalSpecHash: "sha256:technical-spec",
			Status:            state.ReviewPass,
			Summary:           "Technical spec is internally consistent.",
			ReviewedAt:        now,
		},
		execPlan: &state.ExecutionPlan{
			SchemaVersion:           "0.1",
			RunID:                   "run-roundtrip",
			ArtifactType:            "execution_plan",
			SourceTechnicalSpecHash: "sha256:technical-spec",
			Status:                  state.ArtifactDrafted,
			Slices: []state.ExecutionSlice{
				{
					SliceID:            "slice-001",
					Title:              "Add planning artifacts",
					Goal:               "Persist run-scoped planning artifacts.",
					RequirementIDs:     []string{"AT-FR-001"},
					FilesLikelyTouched: []string{"internal/state/types.go"},
					OwnedPaths:         []string{"internal/state"},
					AcceptanceChecks:   []string{"go test ./internal/state"},
					Risk:               "low",
					Size:               "small",
				},
			},
			GeneratedAt: now,
		},
		execReview: &state.ExecutionPlanReview{
			SchemaVersion:     "0.1",
			RunID:             "run-roundtrip",
			ArtifactType:      "execution_plan_review",
			ExecutionPlanHash: "sha256:execution-plan",
			Status:            state.ReviewPass,
			Summary:           "Execution plan is actionable.",
			Warnings: []state.ReviewWarning{{
				WarningID: "epr-w-001",
				SliceID:   "slice-001",
				Summary:   "slice is missing likely file touch points",
			}},
			SharedFileRegistry: []state.SharedFileEntry{{
				File:       "internal/state/types.go",
				Wave1Tasks: []string{"slice-001"},
				Wave2Tasks: []string{"slice-002"},
				Mitigation: "Refresh the shared-file base SHA before starting the later wave.",
			}},
			ReviewedAt: now,
		},
		approval: &state.ArtifactApproval{
			SchemaVersion: "0.1",
			RunID:         "run-roundtrip",
			ArtifactType:  "execution_plan_approval",
			ArtifactPath:  "execution-plan.json",
			ArtifactHash:  "sha256:execution-plan",
			Status:        state.ArtifactApproved,
			ApprovedBy:    "tester",
			ApprovedAt:    approvedAt,
		},
		techApproval: &state.ArtifactApproval{
			SchemaVersion: "0.1",
			RunID:         "run-roundtrip",
			ArtifactType:  "technical_spec_approval",
			ArtifactPath:  "technical-spec.md",
			ArtifactHash:  "sha256:technical-spec",
			Status:        state.ArtifactUnderReview,
			ApprovedBy:    "tester",
			ApprovedAt:    approvedAt,
		},
		techSpecMarkdown: []byte("# Technical Spec\n"),
		execPlanMarkdown: []byte("# Execution Plan\n"),
	}
}

func TestRunArtifactDecodesWithoutNormalizationMetadata(t *testing.T) {
	raw := []byte(`{
		"schema_version": "0.1",
		"run_id": "run-legacy",
		"source_specs": [{"path": "spec.md", "fingerprint": "sha256:spec"}],
		"requirements": [{
			"id": "AT-FR-001",
			"text": "The system must ingest specs.",
			"source_spec": "spec.md",
			"source_line": 12
		}],
		"assumptions": [],
		"clarifications": [],
		"dependencies": [],
		"risk_profile": "low",
		"boundaries": {},
		"routing_policy": {"default_implementer": "sonnet"}
	}`)

	var artifact state.RunArtifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		t.Fatalf("Unmarshal legacy artifact: %v", err)
	}

	if got := artifact.Normalization.Mode; got != "" {
		t.Fatalf("Normalization.Mode = %q, want empty", got)
	}
	if got := artifact.Requirements[0].SourceRefs; len(got) != 0 {
		t.Fatalf("SourceRefs = %+v, want empty", got)
	}
	if got := artifact.Requirements[0].Confidence; got != "" {
		t.Fatalf("Confidence = %q, want empty", got)
	}

	data, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("Marshal legacy artifact: %v", err)
	}
	var remarshal map[string]any
	if err := json.Unmarshal(data, &remarshal); err != nil {
		t.Fatalf("Unmarshal remarshal artifact: %v", err)
	}
	if _, ok := remarshal["normalization"]; ok {
		t.Fatalf("normalization key present in remarshal: %s", data)
	}
}

func TestRunArtifactNormalizationMetadataJSON(t *testing.T) {
	reviewedAt := time.Date(2026, 4, 15, 10, 30, 0, 0, time.UTC)
	approvedAt := reviewedAt.Add(time.Hour)
	artifact := state.RunArtifact{
		SchemaVersion: "0.1",
		RunID:         "run-normalized",
		SourceSpecs: []state.SourceSpec{
			{Path: "spec.md", Fingerprint: "sha256:spec"},
		},
		Requirements: []state.Requirement{{
			ID:         "AT-FR-001",
			Text:       "The system must ingest prose specs.",
			SourceSpec: "spec.md",
			SourceLine: 12,
			SourceRefs: []state.SourceRef{{
				Path:        "spec.md",
				LineStart:   12,
				LineEnd:     14,
				SectionPath: "Goals",
				Excerpt:     "The system must ingest prose specs.",
			}},
			Confidence: "high",
		}},
		Normalization: state.NormalizationMetadata{
			Mode:                  state.NormalizationAuto,
			UsedLLM:               true,
			UsedDeterministic:     false,
			FallbackDeterministic: false,
			ConsentSource:         "flag",
			ConverterBackend:      "converter",
			VerifierBackend:       "verifier",
			ReviewedAt:            &reviewedAt,
			ApprovedAt:            &approvedAt,
			ReviewedInputHash:     "sha256:reviewed",
		},
	}

	data, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("Marshal artifact: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal marshaled artifact: %v", err)
	}

	normalization := got["normalization"].(map[string]any)
	if normalization["mode"] != "auto" {
		t.Fatalf("normalization.mode = %v, want auto", normalization["mode"])
	}
	if normalization["used_llm"] != true {
		t.Fatalf("normalization.used_llm = %v, want true", normalization["used_llm"])
	}
	if normalization["used_deterministic"] != false {
		t.Fatalf("normalization.used_deterministic = %v, want false", normalization["used_deterministic"])
	}
	if normalization["fallback_deterministic"] != false {
		t.Fatalf("normalization.fallback_deterministic = %v, want false", normalization["fallback_deterministic"])
	}
	for key, want := range map[string]string{
		"consent_source":      "flag",
		"converter_backend":   "converter",
		"verifier_backend":    "verifier",
		"reviewed_at":         reviewedAt.Format(time.RFC3339),
		"approved_at":         approvedAt.Format(time.RFC3339),
		"reviewed_input_hash": "sha256:reviewed",
	} {
		if normalization[key] != want {
			t.Fatalf("normalization.%s = %v, want %q", key, normalization[key], want)
		}
	}

	requirements := got["requirements"].([]any)
	requirement := requirements[0].(map[string]any)
	if requirement["confidence"] != "high" {
		t.Fatalf("requirement.confidence = %v, want high", requirement["confidence"])
	}
	sourceRefs := requirement["source_refs"].([]any)
	sourceRef := sourceRefs[0].(map[string]any)
	for key, want := range map[string]any{
		"path":         "spec.md",
		"line_start":   float64(12),
		"line_end":     float64(14),
		"section_path": "Goals",
		"excerpt":      "The system must ingest prose specs.",
	} {
		if sourceRef[key] != want {
			t.Fatalf("source_refs[0].%s = %v, want %v", key, sourceRef[key], want)
		}
	}
}

func TestSpecNormalizationReviewApprovalAndManifestJSONRoundTrip(t *testing.T) {
	reviewedAt := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	approvedAt := reviewedAt.Add(30 * time.Minute)

	review := state.SpecNormalizationReview{
		SchemaVersion:          "0.1",
		RunID:                  "run-normalized",
		ArtifactType:           state.ArtifactTypeSpecNormalizationReview,
		Status:                 state.ReviewNeedsRevision,
		Summary:                "One source requirement is ambiguous.",
		NormalizedArtifactHash: "sha256:artifact",
		SourceManifestHash:     "sha256:manifest",
		ReviewedInputHash:      "sha256:reviewed-input",
		ReviewedAt:             reviewedAt,
		BlockingFindings: []state.ReviewFinding{{
			FindingID:      "snr-001",
			Severity:       "high",
			Category:       "ambiguous_requirement",
			Summary:        "Clarify retention behavior.",
			RequirementIDs: []string{"AT-FR-001"},
		}},
		Warnings: []state.ReviewWarning{{
			WarningID: "snr-w-001",
			Summary:   "Source evidence is broad.",
		}},
	}
	approval := state.RunArtifactApproval{
		SchemaVersion:          "0.1",
		RunID:                  "run-normalized",
		ArtifactType:           state.ArtifactTypeRunArtifactApproval,
		NormalizedArtifactHash: "sha256:artifact",
		SourceManifestHash:     "sha256:manifest",
		ReviewedInputHash:      "sha256:reviewed-input",
		ApprovedAt:             approvedAt,
	}
	manifest := state.SpecNormalizationSourceManifest{
		SchemaVersion: "0.1",
		RunID:         "run-normalized",
		Sources: []state.SourceManifestEntry{{
			Path:        "docs/specs/freeform.md",
			Fingerprint: "sha256:source",
			ByteSize:    4096,
			LineCount:   87,
		}},
	}
	reviewedInput := state.SpecNormalizationReviewedInput{
		SourceManifestHash:     "sha256:manifest",
		NormalizedArtifactHash: "sha256:artifact",
		ConverterPromptHash:    "sha256:converter-prompt",
		VerifierPromptHash:     "sha256:verifier-prompt",
	}

	assertJSONRoundTrip(t, review)
	assertJSONRoundTrip(t, approval)
	assertJSONRoundTrip(t, manifest)
	assertJSONRoundTrip(t, reviewedInput)

	if review.ArtifactType == approval.ArtifactType {
		t.Fatalf("normalization review and run-artifact approval artifact_type values must differ")
	}

	var raw map[string]any
	data, err := json.Marshal(review)
	if err != nil {
		t.Fatalf("Marshal review: %v", err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal review map: %v", err)
	}
	for _, key := range []string{"normalized_artifact_hash", "source_manifest_hash", "reviewed_input_hash"} {
		if raw[key] == "" {
			t.Fatalf("review JSON missing %q: %s", key, data)
		}
	}
	if raw["artifact_type"] != "spec_normalization_review" {
		t.Fatalf("review artifact_type = %v, want spec_normalization_review", raw["artifact_type"])
	}

	data, err = json.Marshal(approval)
	if err != nil {
		t.Fatalf("Marshal approval: %v", err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal approval map: %v", err)
	}
	if raw["artifact_type"] != "run_artifact_approval" {
		t.Fatalf("approval artifact_type = %v, want run_artifact_approval", raw["artifact_type"])
	}
	for _, key := range []string{"normalized_artifact_hash", "source_manifest_hash", "reviewed_input_hash"} {
		if raw[key] == "" {
			t.Fatalf("approval JSON missing %q: %s", key, data)
		}
	}

	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal manifest: %v", err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal manifest map: %v", err)
	}
	sources := raw["sources"].([]any)
	source := sources[0].(map[string]any)
	for key, want := range map[string]any{
		"path":        "docs/specs/freeform.md",
		"fingerprint": "sha256:source",
		"byte_size":   float64(4096),
		"line_count":  float64(87),
	} {
		if source[key] != want {
			t.Fatalf("sources[0].%s = %v, want %v", key, source[key], want)
		}
	}
}

func assertJSONRoundTrip[T any](t *testing.T, want T) {
	t.Helper()

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal %T: %v", want, err)
	}

	var got T
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal %T: %v", want, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip %T = %+v, want %+v", want, got, want)
	}
}

func writeRoundTripFixture(t *testing.T, runDir *state.RunDir, fixture roundTripFixture) {
	t.Helper()

	writes := []struct {
		name string
		run  func() error
	}{
		{name: "WriteArtifact", run: func() error { return runDir.WriteArtifact(fixture.artifact) }},
		{name: "WriteTasks", run: func() error { return runDir.WriteTasks(fixture.tasks) }},
		{name: "WriteCoverage", run: func() error { return runDir.WriteCoverage(fixture.coverage) }},
		{name: "WriteStatus", run: func() error { return runDir.WriteStatus(fixture.status) }},
		{name: "WriteTechnicalSpecReview", run: func() error { return runDir.WriteTechnicalSpecReview(fixture.techReview) }},
		{name: "WriteExecutionPlan", run: func() error { return runDir.WriteExecutionPlan(fixture.execPlan) }},
		{name: "WriteExecutionPlanReview", run: func() error { return runDir.WriteExecutionPlanReview(fixture.execReview) }},
		{name: "WriteExecutionPlanApproval", run: func() error { return runDir.WriteExecutionPlanApproval(fixture.approval) }},
		{name: "WriteTechnicalSpecApproval", run: func() error { return runDir.WriteTechnicalSpecApproval(fixture.techApproval) }},
		{name: "WriteTechnicalSpec", run: func() error { return runDir.WriteTechnicalSpec(fixture.techSpecMarkdown) }},
		{name: "WriteExecutionPlanMarkdown", run: func() error { return runDir.WriteExecutionPlanMarkdown(fixture.execPlanMarkdown) }},
	}

	for _, write := range writes {
		if err := write.run(); err != nil {
			t.Fatalf("%s: %v", write.name, err)
		}
	}
}

func assertRoundTripFixture(t *testing.T, runDir *state.RunDir, fixture roundTripFixture) {
	t.Helper()

	assertRoundTripCoreRecords(t, runDir, fixture)
	assertRoundTripPlanningRecords(t, runDir, fixture)
	assertRoundTripMarkdown(t, runDir, fixture)
}

func assertRoundTripCoreRecords(t *testing.T, runDir *state.RunDir, fixture roundTripFixture) {
	t.Helper()

	gotArtifact, err := runDir.ReadArtifact()
	if err != nil {
		t.Fatalf("ReadArtifact: %v", err)
	}
	if gotArtifact.RunID != fixture.artifact.RunID || gotArtifact.ApprovedBy != fixture.artifact.ApprovedBy {
		t.Fatalf("unexpected artifact: %+v", gotArtifact)
	}

	gotTasks, err := runDir.ReadTasks()
	if err != nil {
		t.Fatalf("ReadTasks: %v", err)
	}
	if len(gotTasks) != 1 || gotTasks[0].TaskID != fixture.tasks[0].TaskID {
		t.Fatalf("unexpected tasks: %+v", gotTasks)
	}

	gotCoverage, err := runDir.ReadCoverage()
	if err != nil {
		t.Fatalf("ReadCoverage: %v", err)
	}
	if len(gotCoverage) != 1 || gotCoverage[0].RequirementID != fixture.coverage[0].RequirementID {
		t.Fatalf("unexpected coverage: %+v", gotCoverage)
	}

	gotStatus, err := runDir.ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}
	if gotStatus.RunID != fixture.status.RunID || gotStatus.TaskCountsByState["pending"] != 1 {
		t.Fatalf("unexpected status: %+v", gotStatus)
	}
}

func assertRoundTripPlanningRecords(t *testing.T, runDir *state.RunDir, fixture roundTripFixture) {
	t.Helper()

	gotTechReview, err := runDir.ReadTechnicalSpecReview()
	if err != nil {
		t.Fatalf("ReadTechnicalSpecReview: %v", err)
	}
	if gotTechReview.RunID != fixture.techReview.RunID || gotTechReview.Status != state.ReviewPass {
		t.Fatalf("unexpected technical spec review: %+v", gotTechReview)
	}

	gotExecPlan, err := runDir.ReadExecutionPlan()
	if err != nil {
		t.Fatalf("ReadExecutionPlan: %v", err)
	}
	if len(gotExecPlan.Slices) != 1 || gotExecPlan.Slices[0].SliceID != "slice-001" {
		t.Fatalf("unexpected execution plan: %+v", gotExecPlan)
	}

	gotExecReview, err := runDir.ReadExecutionPlanReview()
	if err != nil {
		t.Fatalf("ReadExecutionPlanReview: %v", err)
	}
	if gotExecReview.ExecutionPlanHash != fixture.execReview.ExecutionPlanHash || gotExecReview.Status != state.ReviewPass {
		t.Fatalf("unexpected execution plan review: %+v", gotExecReview)
	}
	if !reflect.DeepEqual(gotExecReview.SharedFileRegistry, fixture.execReview.SharedFileRegistry) {
		t.Fatalf("unexpected shared file registry: %+v", gotExecReview.SharedFileRegistry)
	}
	if !reflect.DeepEqual(gotExecReview.Warnings, fixture.execReview.Warnings) {
		t.Fatalf("unexpected warnings: %+v", gotExecReview.Warnings)
	}

	gotApproval, err := runDir.ReadExecutionPlanApproval()
	if err != nil {
		t.Fatalf("ReadExecutionPlanApproval: %v", err)
	}
	if gotApproval.ArtifactHash != fixture.approval.ArtifactHash || gotApproval.Status != state.ArtifactApproved {
		t.Fatalf("unexpected execution plan approval: %+v", gotApproval)
	}

	gotTechApproval, err := runDir.ReadTechnicalSpecApproval()
	if err != nil {
		t.Fatalf("ReadTechnicalSpecApproval: %v", err)
	}
	if gotTechApproval.ArtifactHash != fixture.techApproval.ArtifactHash || gotTechApproval.Status != state.ArtifactUnderReview {
		t.Fatalf("unexpected technical spec approval: %+v", gotTechApproval)
	}
}

func assertRoundTripMarkdown(t *testing.T, runDir *state.RunDir, fixture roundTripFixture) {
	t.Helper()

	techSpecData, err := runDir.ReadTechnicalSpec()
	if err != nil {
		t.Fatalf("ReadTechnicalSpec: %v", err)
	}
	if !bytes.Equal(techSpecData, fixture.techSpecMarkdown) {
		t.Fatalf("unexpected technical spec markdown: %q", string(techSpecData))
	}

	execPlanData, err := runDir.ReadExecutionPlanMarkdown()
	if err != nil {
		t.Fatalf("ReadExecutionPlanMarkdown: %v", err)
	}
	if !bytes.Equal(execPlanData, fixture.execPlanMarkdown) {
		t.Fatalf("unexpected execution plan markdown: %q", string(execPlanData))
	}
}

func TestRunDirReadWriteRoundTrip(t *testing.T) {
	baseDir := t.TempDir()
	runDir := state.NewRunDir(baseDir, "run-roundtrip")
	if err := runDir.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	now := time.Date(2026, time.March, 12, 10, 0, 0, 0, time.UTC)
	approvedAt := now.Add(30 * time.Minute)
	fixture := newRoundTripFixture(now, approvedAt)

	writeRoundTripFixture(t, runDir, fixture)
	assertRoundTripFixture(t, runDir, fixture)
}

func TestRunDirAppendEventWritesJSONLines(t *testing.T) {
	baseDir := t.TempDir()
	runDir := state.NewRunDir(baseDir, "run-events")
	if err := runDir.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	events := []state.Event{
		{
			Timestamp: time.Date(2026, time.March, 12, 11, 0, 0, 0, time.UTC),
			Type:      "run_state_transition",
			RunID:     "run-events",
			Detail:    "approved",
		},
		{
			Timestamp: time.Date(2026, time.March, 12, 11, 1, 0, 0, time.UTC),
			Type:      "tasks_compiled",
			RunID:     "run-events",
			TaskID:    "task-at-fr-001",
			Detail:    "compiled 1 task",
		},
	}

	for i := range events {
		if err := runDir.AppendEvent(events[i]); err != nil {
			t.Fatalf("AppendEvent(%d): %v", i, err)
		}
	}

	file, err := os.Open(runDir.Events())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = file.Close() }()

	var decoded []state.Event
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event state.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("Unmarshal event: %v", err)
		}
		decoded = append(decoded, event)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(decoded) != len(events) {
		t.Fatalf("got %d events, want %d", len(decoded), len(events))
	}
	if decoded[1].TaskID != "task-at-fr-001" || decoded[1].Detail != "compiled 1 task" {
		t.Fatalf("unexpected decoded events: %+v", decoded)
	}
}

func TestRunDirTaskStoreReadWriteTask(t *testing.T) {
	baseDir := t.TempDir()
	runDir := state.NewRunDir(baseDir, "run-store-test")
	if err := runDir.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	store := runDir.AsTaskStore()

	// Write tasks via store.
	tasks := []state.Task{
		{TaskID: "task-1", Title: "First", Status: state.TaskPending},
		{TaskID: "task-2", Title: "Second", Status: state.TaskPending},
	}
	if err := store.WriteTasks("run-store-test", tasks); err != nil {
		t.Fatalf("WriteTasks: %v", err)
	}

	// Read all tasks.
	got, err := store.ReadTasks("run-store-test")
	if err != nil {
		t.Fatalf("ReadTasks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d tasks, want 2", len(got))
	}

	// Read single task.
	task, err := store.ReadTask("task-1")
	if err != nil {
		t.Fatalf("ReadTask: %v", err)
	}
	if task.Title != "First" {
		t.Errorf("Title = %q, want First", task.Title)
	}

	// Read nonexistent.
	_, err = store.ReadTask("nonexistent")
	if err == nil {
		t.Error("ReadTask should error on nonexistent")
	}

	// WriteTask updates existing.
	task.Status = state.TaskDone
	if err := store.WriteTask(task); err != nil {
		t.Fatalf("WriteTask: %v", err)
	}
	updated, _ := store.ReadTask("task-1")
	if updated.Status != state.TaskDone {
		t.Errorf("Status = %q, want done", updated.Status)
	}

	// WriteTask appends new.
	if err := store.WriteTask(&state.Task{TaskID: "task-3", Title: "Third"}); err != nil {
		t.Fatalf("WriteTask (new): %v", err)
	}
	all, _ := store.ReadTasks("run-store-test")
	if len(all) != 3 {
		t.Fatalf("got %d tasks after append, want 3", len(all))
	}
}

func TestRunDirTaskStoreUpdateStatus(t *testing.T) {
	baseDir := t.TempDir()
	runDir := state.NewRunDir(baseDir, "run-status-test")
	if err := runDir.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	store := runDir.AsTaskStore()
	if err := store.WriteTasks("run-status-test", []state.Task{
		{TaskID: "task-1", Status: state.TaskPending},
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.UpdateStatus("task-1", state.TaskDone, "verified"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	task, _ := store.ReadTask("task-1")
	if task.Status != state.TaskDone {
		t.Errorf("Status = %q, want done", task.Status)
	}
	if task.StatusReason != "verified" {
		t.Errorf("StatusReason = %q, want verified", task.StatusReason)
	}
	if task.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}

	// Nonexistent task.
	err := store.UpdateStatus("nonexistent", state.TaskDone, "")
	if err == nil {
		t.Error("UpdateStatus should error on nonexistent")
	}
}

func TestRunDirTaskStoreCreateRunNoop(t *testing.T) {
	baseDir := t.TempDir()
	runDir := state.NewRunDir(baseDir, "run-noop")
	store := runDir.AsTaskStore()
	if err := store.CreateRun("run-noop"); err != nil {
		t.Fatalf("CreateRun should be no-op: %v", err)
	}
}
