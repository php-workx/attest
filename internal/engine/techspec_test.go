package engine

import (
	"strings"
	"testing"
)

func TestNormalizationPromptContainsAllHeadings(t *testing.T) {
	for _, req := range technicalSpecRequirements {
		if !strings.Contains(technicalSpecNormalizationPrompt, req.canonicalHeading) {
			t.Errorf("prompt missing canonical heading %q", req.canonicalHeading)
		}
	}
}

func TestNormalizationPromptContainsPreservationRules(t *testing.T) {
	rules := []struct {
		name    string
		keyword string
	}{
		{"preserve all content", "Preserve ALL content"},
		{"additional content fallback", "Additional Content"},
		{"no invention", "Do not invent"},
		{"preserve formatting", "Preserve code blocks"},
		{"extra sections", "sections beyond the 8 canonical"},
	}
	for _, rule := range rules {
		if !strings.Contains(technicalSpecNormalizationPrompt, rule.keyword) {
			t.Errorf("prompt missing preservation rule %q (looked for %q)", rule.name, rule.keyword)
		}
	}
}

func TestNormalizationPromptContainsOutputInstruction(t *testing.T) {
	if !strings.Contains(technicalSpecNormalizationPrompt, "Return ONLY") {
		t.Error("prompt missing output instruction 'Return ONLY'")
	}
}

func TestNormalizationPromptNoPlaceholders(t *testing.T) {
	for _, placeholder := range []string{"TODO", "FIXME", "HACK", "XXX"} {
		if strings.Contains(technicalSpecNormalizationPrompt, placeholder) {
			t.Errorf("prompt contains placeholder %q", placeholder)
		}
	}
}

func TestCountMarkdownSections(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"empty", "", 0},
		{"no sections", "just text\nmore text", 0},
		{"one section", "## Heading\ncontent", 1},
		{"three sections", "## A\n## B\n## C\n", 3},
		{"h3 not counted", "### Sub\n## Main\n", 1},
		{"indented heading", "  ## Indented\n", 1},
		{"h1 not counted", "# Title\n## Section\n", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countMarkdownSections(tt.text)
			if got != tt.want {
				t.Errorf("countMarkdownSections() = %d, want %d", got, tt.want)
			}
		})
	}
}
