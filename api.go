package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	maxContextTokens = 128000
	maxHistoryLength = 100
	maxToolOutput    = 50000
	maxFileRead      = 1 * 1024 * 1024 // 1 MB cap for read_file tool
	maxErrorBody     = 4 * 1024        // 4 KB cap for HTTP error response bodies
	maxAgentsMDSize  = 64 * 1024       // 64 KB cap per AGENTS.md / SKILL.md file
)

var httpClient = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: false},
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  true,
		MaxConnsPerHost:     5,
		ResponseHeaderTimeout: 30 * time.Second,
	},
}

type Message struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	ToolCalls  []ToolCallMsg `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type ToolCallMsg struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type chatRequest struct {
	Model           string    `json:"model"`
	Messages        []Message `json:"messages"`
	Stream          bool      `json:"stream"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
	Tools           []Tool    `json:"tools,omitempty"`
}

func reasoningEffort(level int) string {
	switch {
	case level >= 8:
		return "high"
	case level >= 4:
		return "medium"
	case level >= 1:
		return "low"
	default:
		return ""
	}
}

type toolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string          `json:"content"`
			ToolCalls []toolCallDelta `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func complete(cfg Config, msgs []Message, tools []Tool, onFirstToken func(), onToken func(string)) (string, []ToolCallMsg, error) {
	body, err := json.Marshal(chatRequest{
		Model:           cfg.Model,
		Messages:        msgs,
		Stream:          true,
		ReasoningEffort: reasoningEffort(cfg.Thinking),
		Tools:           tools,
	})
	if err != nil {
		return "", nil, err
	}

	req, err := http.NewRequest("POST", cfg.BaseURL+"/chat/completions", nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("User-Agent", "au/alpha")

	// Retry logic with exponential backoff; reset body each attempt so retries send full payload
	var resp *http.Response
	var httpErr error
	for attempt := 0; attempt < 3; attempt++ {
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		resp, httpErr = httpClient.Do(req)
		if httpErr == nil {
			break
		}
		if attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
	}
	if httpErr != nil {
		return "", nil, httpErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		buf.ReadFrom(io.LimitReader(resp.Body, maxErrorBody))
		errMsg := buf.String()
		// Strip sensitive data from error message
		errMsg = strings.ReplaceAll(errMsg, cfg.APIKey, "***REDACTED***")
		errMsg = strings.ReplaceAll(errMsg, "Bearer "+cfg.APIKey, "Bearer ***REDACTED***")
		return "", nil, fmt.Errorf("api error %d: %s", resp.StatusCode, errMsg)
	}

	var out strings.Builder
	first := true
	tcMap := map[int]*ToolCallMsg{}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 4*1024*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			// Skip non-SSE lines but continue
			continue
		}
		data := line[6:]
		if data == "[DONE]" {
			break
		}
		// Validate data is not empty before parsing
		if strings.TrimSpace(data) == "" {
			continue
		}
		var c streamChunk
		if err := json.Unmarshal([]byte(data), &c); err != nil {
			// Log parsing error but continue to avoid breaking stream
			continue
		}
		if len(c.Choices) == 0 {
			continue
		}
		ch := c.Choices[0]

		tok := ch.Delta.Content
		if tok != "" {
			if first {
				first = false
				if onFirstToken != nil {
					onFirstToken()
				}
			}
			if onToken != nil {
				onToken(tok)
			}
			out.WriteString(tok)
		}

		for _, tcd := range ch.Delta.ToolCalls {
			if first {
				first = false
				if onFirstToken != nil {
					onFirstToken()
				}
			}
			tc := tcMap[tcd.Index]
			if tc == nil {
				tc = &ToolCallMsg{Type: "function"}
				tcMap[tcd.Index] = tc
			}
			if tcd.ID != "" {
				tc.ID = tcd.ID
			}
			if tcd.Function.Name != "" {
				tc.Function.Name = tcd.Function.Name
			}
			tc.Function.Arguments += tcd.Function.Arguments
		}
	}

	if first && onFirstToken != nil {
		onFirstToken()
	}

	// Validate and truncate tool calls by index
	toolCalls := make([]ToolCallMsg, 0, len(tcMap))
	for i := 0; i < len(tcMap); i++ {
		if tc, ok := tcMap[i]; ok {
			// Validate tool call before adding
			if tc.Function.Name == "" || tc.ID == "" {
				continue
			}
			toolCalls = append(toolCalls, *tc)
		}
	}

	return out.String(), toolCalls, sc.Err()
}

type modelEntry struct {
	ID string `json:"id"`
}

type modelsResp struct {
	Data []modelEntry `json:"data"`
}

func listModels(cfg Config) ([]string, error) {
	req, err := http.NewRequest("GET", cfg.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("User-Agent", "au/alpha")

	// Retry logic with exponential backoff
	var resp *http.Response
	var httpErr error
	for attempt := 0; attempt < 3; attempt++ {
		resp, httpErr = httpClient.Do(req)
		if httpErr == nil {
			break
		}
		if attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
	}
	if httpErr != nil {
		return nil, httpErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		buf.ReadFrom(io.LimitReader(resp.Body, maxErrorBody))
		errMsg := buf.String()
		// Strip sensitive data from error message
		errMsg = strings.ReplaceAll(errMsg, cfg.APIKey, "***REDACTED***")
		errMsg = strings.ReplaceAll(errMsg, "Bearer "+cfg.APIKey, "Bearer ***REDACTED***")
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, errMsg)
	}

	var mr modelsResp
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, err
	}

	out := make([]string, 0, len(mr.Data))
	for _, m := range mr.Data {
		out = append(out, m.ID)
	}
	return out, nil
}
