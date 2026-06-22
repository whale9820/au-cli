package main

import "os"

type Config struct {
	BaseURL  string
	APIKey   string
	Model    string
	Thinking int
}

func loadConfig(s *Store) Config {
	baseURL := os.Getenv("AU_BASE_URL")
	if baseURL == "" {
		baseURL = s.BaseURL
	}
	if baseURL == "" {
		baseURL = fallbackProviders[0].BaseURL
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	apiKey := os.Getenv("AU_API_KEY")
	if apiKey == "" {
		apiKey = s.APIKey
	}
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	model := os.Getenv("AU_MODEL")
	if model == "" {
		model = s.Model
	}
	if model == "" {
		model = ""
	}

	thinking := s.Thinking
	if thinking < 0 || thinking > 10 {
		thinking = 0
	}

	return Config{BaseURL: s.resolve(baseURL), APIKey: apiKey, Model: model, Thinking: thinking}
}
