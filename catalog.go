package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const modelsDevURL = "https://models.dev/api.json"

type ModelSpec struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Context     int    `json:"context,omitempty"`
	Output      int    `json:"output,omitempty"`
	Tools       *bool  `json:"tools,omitempty"`
	Reasoning   *bool  `json:"reasoning,omitempty"`
	Vision      *bool  `json:"vision,omitempty"`
	FromCatalog bool   `json:"from_catalog,omitempty"`
}

type Provider struct {
	Name    string      `json:"name"`
	BaseURL string      `json:"base_url"`
	Tag     string      `json:"tag,omitempty"`
	Env     []string    `json:"env,omitempty"`
	Model   string      `json:"model,omitempty"`
	Spec    *ModelSpec  `json:"spec,omitempty"`
	Models  []ModelSpec `json:"models,omitempty"`
	Custom  bool        `json:"custom,omitempty"`
}

type ProviderCatalog struct {
	Providers []Provider
	Fallback  bool
	Err       error
}

type modelsDevProvider struct {
	ID     string                    `json:"id"`
	Name   string                    `json:"name"`
	API    any                       `json:"api"`
	Env    []string                  `json:"env"`
	Models map[string]modelsDevModel `json:"models"`
}

type modelsDevModel struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	ToolCall   bool     `json:"tool_call"`
	Reasoning  bool     `json:"reasoning"`
	Modalities modality `json:"modalities"`
	Limit      limit    `json:"limit"`
}

type modality struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

type limit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

var fallbackProviders = []Provider{
	{Name: "OpenAI", BaseURL: "https://api.openai.com/v1", Tag: "API", Env: []string{"OPENAI_API_KEY"}},
	{Name: "Anthropic", BaseURL: "https://api.anthropic.com/v1", Tag: "API", Env: []string{"ANTHROPIC_API_KEY"}},
	{Name: "Google Gemini", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai", Tag: "API", Env: []string{"GEMINI_API_KEY"}},
	{Name: "xAI (Grok)", BaseURL: "https://api.x.ai/v1", Tag: "API", Env: []string{"XAI_API_KEY"}},
	{Name: "DeepSeek", BaseURL: "https://api.deepseek.com/v1", Tag: "API", Env: []string{"DEEPSEEK_API_KEY"}},
	{Name: "Mistral AI", BaseURL: "https://api.mistral.ai/v1", Tag: "API", Env: []string{"MISTRAL_API_KEY"}},
	{Name: "Groq", BaseURL: "https://api.groq.com/openai/v1", Tag: "API", Env: []string{"GROQ_API_KEY"}},
	{Name: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1", Tag: "API", Env: []string{"OPENROUTER_API_KEY"}},
	{Name: "Cloudflare Workers AI", BaseURL: "https://api.cloudflare.com/client/v4/accounts/{ACCOUNT_ID}/ai/v1", Tag: "API"},
	{Name: "Ollama", BaseURL: "http://localhost:11434/v1", Tag: "Local"},
	{Name: "LM Studio", BaseURL: "http://localhost:1234/v1", Tag: "Local"},
}

func loadProviderCatalog(custom []Provider) ProviderCatalog {
	providers, err := fetchModelsDev(modelsDevURL)
	cat := ProviderCatalog{Providers: providers}
	if err != nil || len(providers) == 0 {
		cat = ProviderCatalog{Providers: append([]Provider(nil), fallbackProviders...), Fallback: true, Err: err}
	}
	cat.Providers = appendCustomProviders(cat.Providers, custom)
	return cat
}

func fetchModelsDev(url string) ([]Provider, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "au/alpha")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models.dev returned %d", resp.StatusCode)
	}
	var data map[string]modelsDevProvider
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return parseModelsDev(data), nil
}

func parseModelsDev(data map[string]modelsDevProvider) []Provider {
	out := make([]Provider, 0, len(data))
	for _, p := range data {
		api, ok := p.API.(string)
		if !ok || strings.TrimSpace(api) == "" {
			continue
		}
		name := p.Name
		if name == "" {
			name = p.ID
		}
		prov := Provider{Name: name, BaseURL: api, Tag: "models.dev", Env: p.Env}
		for id, m := range p.Models {
			ms := modelSpecFromModelsDev(id, m)
			prov.Models = append(prov.Models, ms)
		}
		sort.Slice(prov.Models, func(i, j int) bool { return prov.Models[i].ID < prov.Models[j].ID })
		out = append(out, prov)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func modelSpecFromModelsDev(id string, m modelsDevModel) ModelSpec {
	if m.ID != "" {
		id = m.ID
	}
	tools, reasoning, vision := m.ToolCall, m.Reasoning, contains(m.Modalities.Input, "image")
	return ModelSpec{
		ID:          id,
		Name:        m.Name,
		Context:     m.Limit.Context,
		Output:      m.Limit.Output,
		Tools:       &tools,
		Reasoning:   &reasoning,
		Vision:      &vision,
		FromCatalog: true,
	}
}

func appendCustomProviders(providers []Provider, custom []Provider) []Provider {
	out := append([]Provider(nil), providers...)
	for _, p := range custom {
		p.Custom = true
		if p.Tag == "" {
			p.Tag = "Custom"
		}
		out = append(out, p)
	}
	return out
}

func (c ProviderCatalog) findProvider(name string) *Provider {
	lower := strings.ToLower(name)
	for i := range c.Providers {
		if strings.ToLower(c.Providers[i].Name) == lower {
			return &c.Providers[i]
		}
	}
	for i := range c.Providers {
		if strings.Contains(strings.ToLower(c.Providers[i].Name), lower) {
			return &c.Providers[i]
		}
	}
	return nil
}

func (c ProviderCatalog) findModel(providerName, modelID string) *ModelSpec {
	p := c.findProvider(providerName)
	if p == nil {
		return nil
	}
	return p.findModel(modelID)
}

func (p Provider) findModel(modelID string) *ModelSpec {
	for i := range p.Models {
		if p.Models[i].ID == modelID || strings.EqualFold(p.Models[i].Name, modelID) {
			return &p.Models[i]
		}
	}
	return nil
}

func mergeModelSpec(base *ModelSpec, override *ModelSpec) *ModelSpec {
	if base == nil && override == nil {
		return nil
	}
	out := ModelSpec{}
	if base != nil {
		out = *base
	}
	if override == nil {
		return &out
	}
	if override.ID != "" {
		out.ID = override.ID
	}
	if override.Name != "" {
		out.Name = override.Name
	}
	if override.Context != 0 {
		out.Context = override.Context
	}
	if override.Output != 0 {
		out.Output = override.Output
	}
	if override.Tools != nil {
		out.Tools = override.Tools
	}
	if override.Reasoning != nil {
		out.Reasoning = override.Reasoning
	}
	if override.Vision != nil {
		out.Vision = override.Vision
	}
	return &out
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
