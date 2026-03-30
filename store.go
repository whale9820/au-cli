package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
)

type Store struct {
	BaseURL  string            `json:"base_url,omitempty"`
	APIKey   string            `json:"api_key,omitempty"`
	Model    string            `json:"model,omitempty"`
	Thinking int               `json:"thinking,omitempty"`
	Vars     map[string]string `json:"vars,omitempty"`
}

var rePlaceholder = regexp.MustCompile(`\{([^}]+)\}`)

func configPath() string {
	dir, _ := os.UserConfigDir()
	if dir == "" {
		dir = os.Getenv("HOME")
	}
	return filepath.Join(dir, "au", "config.json")
}

func loadStore() *Store {
	s := &Store{Vars: map[string]string{}}
	if data, err := os.ReadFile(configPath()); err == nil {
		json.Unmarshal(data, s)
	}
	if s.Vars == nil {
		s.Vars = map[string]string{}
	}
	return s
}

func (s *Store) save() {
	p := configPath()
	os.MkdirAll(filepath.Dir(p), 0755)
	if data, err := json.MarshalIndent(s, "", "  "); err == nil {
		os.WriteFile(p, data, 0644)
	}
}

func (s *Store) resolve(url string) string {
	return rePlaceholder.ReplaceAllStringFunc(url, func(m string) string {
		if v := s.Vars[m[1:len(m)-1]]; v != "" {
			return v
		}
		return m
	})
}
