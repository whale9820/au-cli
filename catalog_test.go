package main

import "testing"

func TestParseModelsDev(t *testing.T) {
	data := map[string]modelsDevProvider{
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			API:  "https://api.openai.com/v1",
			Env:  []string{"OPENAI_API_KEY"},
			Models: map[string]modelsDevModel{
				"gpt-test": {
					ID:         "gpt-test",
					Name:       "GPT Test",
					ToolCall:   true,
					Reasoning:  true,
					Modalities: modality{Input: []string{"text", "image"}, Output: []string{"text"}},
					Limit:      limit{Context: 128000, Output: 16384},
				},
			},
		},
		"anthropic": {ID: "anthropic", Name: "Anthropic", API: nil},
	}

	providers := parseModelsDev(data)
	if len(providers) != 1 {
		t.Fatalf("len(providers) = %d, want 1", len(providers))
	}
	p := providers[0]
	if p.Name != "OpenAI" || p.BaseURL != "https://api.openai.com/v1" || p.Tag != "models.dev" {
		t.Fatalf("provider = %#v", p)
	}
	if len(p.Models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(p.Models))
	}
	m := p.Models[0]
	if m.ID != "gpt-test" || m.Context != 128000 || m.Output != 16384 {
		t.Fatalf("model = %#v", m)
	}
	if m.Tools == nil || !*m.Tools || m.Reasoning == nil || !*m.Reasoning || m.Vision == nil || !*m.Vision {
		t.Fatalf("capabilities not parsed: %#v", m)
	}
}

func TestProviderLookupAndCustomAppend(t *testing.T) {
	cat := ProviderCatalog{Providers: appendCustomProviders([]Provider{{Name: "OpenAI", BaseURL: "https://api.openai.com/v1"}}, []Provider{{Name: "Mine", BaseURL: "https://mine.test/v1"}})}
	if got := cat.findProvider("open"); got == nil || got.Name != "OpenAI" {
		t.Fatalf("find open = %#v", got)
	}
	if got := cat.findProvider("mine"); got == nil || !got.Custom || got.Tag != "Custom" {
		t.Fatalf("find custom = %#v", got)
	}
}

func TestMergeModelSpec(t *testing.T) {
	tools := true
	reasoning := false
	base := &ModelSpec{ID: "m", Context: 100, Output: 10, Tools: &tools}
	override := &ModelSpec{Context: 200, Reasoning: &reasoning}
	got := mergeModelSpec(base, override)
	if got.Context != 200 || got.Output != 10 {
		t.Fatalf("merged limits = %#v", got)
	}
	if got.Tools == nil || !*got.Tools || got.Reasoning == nil || *got.Reasoning {
		t.Fatalf("merged capabilities = %#v", got)
	}
}
