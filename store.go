package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

type Store struct {
	BaseURL    string            `json:"base_url,omitempty"`
	APIKey     string            `json:"api_key,omitempty"`
	Model      string            `json:"model,omitempty"`
	Thinking   int               `json:"thinking,omitempty"`
	Vars       map[string]string `json:"vars,omitempty"`
	History    []string          `json:"history,omitempty"`
	MaxHistory int               `json:"max_history,omitempty"`
}

var rePlaceholder = regexp.MustCompile(`\{([^}]+)\}`)

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = os.Getenv("HOME")
		if dir == "" {
			dir = os.Getenv("USERPROFILE")
			if dir == "" {
				dir = "."
			}
		}
	}
	return filepath.Join(dir, "au", "config.json")
}

func loadStore() *Store {
	s := &Store{
		Vars:       map[string]string{},
		History:    []string{},
		MaxHistory: 100,
	}
	data, err := os.ReadFile(configPath())
	if err != nil {
		// Config file doesn't exist yet, use defaults
		return s
	}
	if err := json.Unmarshal(data, s); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing config: %v, using defaults\n", err)
		return s
	}
	if s.Vars == nil {
		s.Vars = map[string]string{}
	}
	if s.History == nil {
		s.History = []string{}
	}
	if s.MaxHistory <= 0 {
		s.MaxHistory = 100
	}
	return s
}

func (s *Store) save() {
	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0750); err != nil {
		fmt.Fprintf(os.Stderr, "error creating config directory: %v\n", err)
		return
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error encoding config: %v\n", err)
		return
	}
	if err := os.WriteFile(p, data, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "error writing config: %v\n", err)
		return
	}
}

func (s *Store) saveHistory(command string) {
	s.History = append(s.History, command)
	// Trim history to max length
	if len(s.History) > s.MaxHistory {
		s.History = s.History[len(s.History)-s.MaxHistory:]
	}
}

func (s *Store) resolve(url string) string {
	return rePlaceholder.ReplaceAllStringFunc(url, func(m string) string {
		key := m[1 : len(m)-1]
		if v := s.Vars[key]; v != "" {
			return v
		}
		// Return original placeholder if not resolved
		return m
	})
}
