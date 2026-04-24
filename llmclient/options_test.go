package llmclient_test

import (
	"errors"
	"testing"

	"github.com/php-workx/fabrikk/llmclient"
)

func TestDefaultRequestConfig(t *testing.T) {
	cfg := llmclient.DefaultRequestConfig()

	if cfg.Model != "" {
		t.Errorf("expected empty Model, got %q", cfg.Model)
	}
	if cfg.SessionID != "" {
		t.Errorf("expected empty SessionID, got %q", cfg.SessionID)
	}
	if cfg.TemperatureSet {
		t.Error("expected TemperatureSet to be false")
	}
	if cfg.Ollama != nil {
		t.Error("expected nil Ollama")
	}
	if cfg.CodexProfile != "" {
		t.Errorf("expected empty CodexProfile, got %q", cfg.CodexProfile)
	}
	if len(cfg.HostTools) != 0 {
		t.Errorf("expected empty HostTools, got %d entries", len(cfg.HostTools))
	}
	if cfg.OpenCodePort != 0 {
		t.Errorf("expected OpenCodePort 0, got %d", cfg.OpenCodePort)
	}
	if cfg.RequiredOptions == nil {
		t.Error("expected RequiredOptions map to be initialised, got nil")
	}
}

func TestWithModel(t *testing.T) {
	cfg := apply(llmclient.WithModel("claude-opus-4-5"))
	if cfg.Model != "claude-opus-4-5" {
		t.Errorf("expected model %q, got %q", "claude-opus-4-5", cfg.Model)
	}
}

func TestWithSession(t *testing.T) {
	cfg := apply(llmclient.WithSession("sess-abc"))
	if cfg.SessionID != "sess-abc" {
		t.Errorf("expected session %q, got %q", "sess-abc", cfg.SessionID)
	}
}

func TestWithTemperature(t *testing.T) {
	cfg := apply(llmclient.WithTemperature(0.7))
	if !cfg.TemperatureSet {
		t.Error("expected TemperatureSet to be true after WithTemperature")
	}
	if cfg.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %f", cfg.Temperature)
	}
}

func TestWithTemperatureZero(t *testing.T) {
	// Explicitly setting temperature to 0.0 must still mark TemperatureSet.
	cfg := apply(llmclient.WithTemperature(0.0))
	if !cfg.TemperatureSet {
		t.Error("expected TemperatureSet to be true even when temperature is 0")
	}
	if cfg.Temperature != 0.0 {
		t.Errorf("expected temperature 0.0, got %f", cfg.Temperature)
	}
}

func TestWithOllama(t *testing.T) {
	ollamaCfg := llmclient.OllamaConfig{
		BaseURL: "http://localhost:11434",
		Model:   "qwen3.5",
	}
	cfg := apply(llmclient.WithOllama(ollamaCfg))
	if cfg.Ollama == nil {
		t.Fatal("expected non-nil Ollama config")
	}
	if cfg.Ollama.BaseURL != ollamaCfg.BaseURL {
		t.Errorf("expected BaseURL %q, got %q", ollamaCfg.BaseURL, cfg.Ollama.BaseURL)
	}
	if cfg.Ollama.Model != ollamaCfg.Model {
		t.Errorf("expected Model %q, got %q", ollamaCfg.Model, cfg.Ollama.Model)
	}
}

func TestWithCodexProfile(t *testing.T) {
	cfg := apply(llmclient.WithCodexProfile("my-profile"))
	if cfg.CodexProfile != "my-profile" {
		t.Errorf("expected CodexProfile %q, got %q", "my-profile", cfg.CodexProfile)
	}
}

func TestWithHostTools(t *testing.T) {
	tools := []llmclient.Tool{
		{Name: "Read", Description: "Read a file"},
		{Name: "Bash", Description: "Run a shell command"},
	}
	cfg := apply(llmclient.WithHostTools(tools))
	if len(cfg.HostTools) != len(tools) {
		t.Fatalf("expected %d host tools, got %d", len(tools), len(cfg.HostTools))
	}
	for i, want := range tools {
		if cfg.HostTools[i].Name != want.Name {
			t.Errorf("tool[%d]: expected name %q, got %q", i, want.Name, cfg.HostTools[i].Name)
		}
	}
}

func TestWithOpenCodePort(t *testing.T) {
	cfg := apply(llmclient.WithOpenCodePort(8080))
	if cfg.OpenCodePort != 8080 {
		t.Errorf("expected OpenCodePort 8080, got %d", cfg.OpenCodePort)
	}
}

func TestWithRequiredOptions(t *testing.T) {
	cfg := apply(llmclient.WithRequiredOptions(llmclient.OptionModel, llmclient.OptionSession))

	for _, name := range []llmclient.OptionName{llmclient.OptionModel, llmclient.OptionSession} {
		if _, ok := cfg.RequiredOptions[name]; !ok {
			t.Errorf("expected %q to be in RequiredOptions", name)
		}
	}
	// An option not listed must not appear.
	if _, ok := cfg.RequiredOptions[llmclient.OptionTemperature]; ok {
		t.Error("expected OptionTemperature NOT to be in RequiredOptions")
	}
}

// TestRequiredUnsupportedOptionPrecondition verifies that RequiredOptions does
// not silently accept unknown option names — it stores them verbatim so that
// backends can check them against their own capability maps.
func TestRequiredUnsupportedOptionPrecondition(t *testing.T) {
	customOpt := llmclient.OptionName("custom_option")
	cfg := apply(llmclient.WithRequiredOptions(customOpt))

	if _, ok := cfg.RequiredOptions[customOpt]; !ok {
		t.Errorf("expected custom option %q to be stored in RequiredOptions", customOpt)
	}
}

// TestRequiredUnsupportedOptionFailsBeforeSpawn verifies that EnforceRequired
// returns ErrUnsupportedOption when a required option is absent from or
// explicitly unsupported by the backend's capability map.
func TestRequiredUnsupportedOptionFailsBeforeSpawn(t *testing.T) {
	// temperature is required but the backend has no OptionSupport entry for it.
	cfg := apply(llmclient.WithRequiredOptions(llmclient.OptionTemperature))

	capsNoSupport := llmclient.Capabilities{
		Backend:   "stub",
		Streaming: llmclient.StreamingBufferedOnly,
		OptionSupport: map[llmclient.OptionName]llmclient.OptionSupport{
			llmclient.OptionModel: llmclient.OptionSupportFull,
			// OptionTemperature intentionally absent
		},
	}
	if err := llmclient.EnforceRequired(cfg, capsNoSupport); err == nil {
		t.Error("expected ErrUnsupportedOption when required option is absent from caps, got nil")
	} else if !errors.Is(err, llmclient.ErrUnsupportedOption) {
		t.Errorf("expected ErrUnsupportedOption, got %v", err)
	}

	// Explicit OptionSupportNone must also fail.
	capsExplicitNone := llmclient.Capabilities{
		Backend:   "stub",
		Streaming: llmclient.StreamingBufferedOnly,
		OptionSupport: map[llmclient.OptionName]llmclient.OptionSupport{
			llmclient.OptionTemperature: llmclient.OptionSupportNone,
		},
	}
	if err := llmclient.EnforceRequired(cfg, capsExplicitNone); err == nil {
		t.Error("expected ErrUnsupportedOption when required option has OptionSupportNone, got nil")
	} else if !errors.Is(err, llmclient.ErrUnsupportedOption) {
		t.Errorf("expected ErrUnsupportedOption, got %v", err)
	}

	// A backend that does support the required option must not fail.
	capsSupported := llmclient.Capabilities{
		Backend:   "stub",
		Streaming: llmclient.StreamingBufferedOnly,
		OptionSupport: map[llmclient.OptionName]llmclient.OptionSupport{
			llmclient.OptionTemperature: llmclient.OptionSupportFull,
		},
	}
	if err := llmclient.EnforceRequired(cfg, capsSupported); err != nil {
		t.Errorf("expected no error when required option is fully supported, got %v", err)
	}

	// Partial support must also pass (the option is applied, possibly degraded).
	capsPartial := llmclient.Capabilities{
		Backend:   "stub",
		Streaming: llmclient.StreamingBufferedOnly,
		OptionSupport: map[llmclient.OptionName]llmclient.OptionSupport{
			llmclient.OptionTemperature: llmclient.OptionSupportPartial,
		},
	}
	if err := llmclient.EnforceRequired(cfg, capsPartial); err != nil {
		t.Errorf("expected no error when required option has partial support, got %v", err)
	}

	// No required options: EnforceRequired must always return nil.
	cfgNone := apply()
	if err := llmclient.EnforceRequired(cfgNone, capsNoSupport); err != nil {
		t.Errorf("expected no error with empty RequiredOptions, got %v", err)
	}
}

func TestApplyOptionsIsolation(t *testing.T) {
	// Applying options to a shared base must not mutate the original.
	base := llmclient.DefaultRequestConfig()
	base.Model = "original"

	_ = llmclient.ApplyOptions(base, []llmclient.Option{
		llmclient.WithModel("overridden"),
		llmclient.WithRequiredOptions(llmclient.OptionModel),
	})

	if base.Model != "original" {
		t.Errorf("ApplyOptions mutated the base RequestConfig (Model changed to %q)", base.Model)
	}
	if len(base.RequiredOptions) != 0 {
		t.Errorf("ApplyOptions mutated the base RequestConfig (RequiredOptions has entries)")
	}
}

func TestApplyOptionsChaining(t *testing.T) {
	cfg := llmclient.ApplyOptions(llmclient.DefaultRequestConfig(), []llmclient.Option{
		llmclient.WithModel("sonnet"),
		llmclient.WithSession("s1"),
		llmclient.WithTemperature(0.5),
		llmclient.WithOpenCodePort(9090),
	})

	if cfg.Model != "sonnet" {
		t.Errorf("Model: want %q, got %q", "sonnet", cfg.Model)
	}
	if cfg.SessionID != "s1" {
		t.Errorf("SessionID: want %q, got %q", "s1", cfg.SessionID)
	}
	if cfg.Temperature != 0.5 {
		t.Errorf("Temperature: want 0.5, got %f", cfg.Temperature)
	}
	if cfg.OpenCodePort != 9090 {
		t.Errorf("OpenCodePort: want 9090, got %d", cfg.OpenCodePort)
	}
}

// TestApplyOptions_NilOptionIsIgnored verifies that a nil entry in the opts
// slice is silently skipped rather than causing a nil-pointer panic.
func TestApplyOptions_NilOptionIsIgnored(t *testing.T) {
	base := llmclient.DefaultRequestConfig()
	base.Model = "initial"

	// A nil Option must not panic and must not alter any field.
	cfg := llmclient.ApplyOptions(base, []llmclient.Option{
		nil,
		llmclient.WithModel("updated"),
		nil,
	})

	if cfg.Model != "updated" {
		t.Errorf("Model = %q, want %q", cfg.Model, "updated")
	}
}

// TestEnforceRequired_NilOptionSupportMap verifies that EnforceRequired returns
// ErrUnsupportedOption when the capabilities have a nil OptionSupport map and
// RequiredOptions is non-empty. A nil map is valid per the struct definition
// (omitempty) and must behave the same as an empty map.
func TestEnforceRequired_NilOptionSupportMap(t *testing.T) {
	cfg := apply(llmclient.WithRequiredOptions(llmclient.OptionModel))

	capsNilMap := llmclient.Capabilities{
		Backend:       "stub",
		Streaming:     llmclient.StreamingBufferedOnly,
		OptionSupport: nil, // intentionally omitted
	}

	err := llmclient.EnforceRequired(cfg, capsNilMap)
	if err == nil {
		t.Fatal("expected ErrUnsupportedOption when OptionSupport is nil and option is required, got nil")
	}
	if !errors.Is(err, llmclient.ErrUnsupportedOption) {
		t.Errorf("expected ErrUnsupportedOption, got %v", err)
	}
}

// TestEnforceRequired_NilOptionSupportMap_NoRequirements verifies that
// EnforceRequired returns nil when RequiredOptions is empty even if
// OptionSupport is nil (nothing to validate).
func TestEnforceRequired_NilOptionSupportMap_NoRequirements(t *testing.T) {
	cfg := apply() // no WithRequiredOptions

	capsNilMap := llmclient.Capabilities{
		Backend:       "stub",
		Streaming:     llmclient.StreamingBufferedOnly,
		OptionSupport: nil,
	}

	if err := llmclient.EnforceRequired(cfg, capsNilMap); err != nil {
		t.Errorf("expected nil error with empty RequiredOptions and nil OptionSupport, got %v", err)
	}
}

// apply is a helper that creates a default config and applies opts.
func apply(opts ...llmclient.Option) llmclient.RequestConfig {
	return llmclient.ApplyOptions(llmclient.DefaultRequestConfig(), opts)
}
