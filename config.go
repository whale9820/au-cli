package main

import "os"

type Provider struct {
	Name    string
	BaseURL string
	Tag     string
}

var providers = []Provider{
	{"OpenAI",                   "https://api.openai.com/v1",                                                   "API"},
	{"Anthropic",                "https://api.anthropic.com/v1",                                               "API"},
	{"Google Gemini",            "https://generativelanguage.googleapis.com/v1beta/openai",                     "API"},
	{"xAI (Grok)",               "https://api.x.ai/v1",                                                        "API"},
	{"DeepSeek",                 "https://api.deepseek.com/v1",                                                 "API"},
	{"Mistral AI",               "https://api.mistral.ai/v1",                                                   "API"},
	{"Groq",                     "https://api.groq.com/openai/v1",                                             "API"},
	{"Together AI",              "https://api.together.xyz/v1",                                                 "API"},
	{"Fireworks AI",             "https://api.fireworks.ai/inference/v1",                                       "API"},
	{"Perplexity",               "https://api.perplexity.ai",                                                   "API"},
	{"Cerebras",                 "https://api.cerebras.ai/v1",                                                  "API"},
	{"SambaNova",                "https://api.sambanova.ai/v1",                                                 "API"},
	{"Hyperbolic",               "https://api.hyperbolic.xyz/v1",                                              "API"},
	{"Nebius AI Studio",         "https://api.studio.nebius.ai/v1",                                            "API"},
	{"Nebius Token Factory",     "https://api.tokenfactory.nebius.com/v1",                                     "API"},
	{"Novita AI",                "https://api.novita.ai/openai/v1",                                            "API"},
	{"OpenRouter",               "https://openrouter.ai/api/v1",                                               "API"},
	{"Cloudflare Workers AI",    "https://api.cloudflare.com/client/v4/accounts/{ACCOUNT_ID}/ai/v1",           "API"},
	{"GitHub Models",            "https://models.inference.ai.azure.com",                                      "API"},
	{"Hugging Face",             "https://router.huggingface.co/v1",                                           "API"},
	{"Cohere",                   "https://api.cohere.com/compatibility/v1",                                    "API"},
	{"AI21",                     "https://api.ai21.com/studio/v1",                                             "API"},
	{"Scaleway",                 "https://api.scaleway.ai/v1",                                                 "API"},
	{"DeepInfra",                "https://api.deepinfra.com/v1/openai",                                        "API"},
	{"Replicate",                "https://openai-compat.replicate.com/v1",                                     "API"},
	{"Moonshot AI (Kimi)",       "https://api.moonshot.cn/v1",                                                 "API"},
	{"Qwen / Alibaba Cloud",     "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",                     "API"},
	{"BigModel",                 "https://open.bigmodel.cn/api/paas/v4",                                       "API"},
	{"NLP Cloud",                "https://api.nlpcloud.io/v1/gpu",                                             "API"},
	{"Z.AI",                     "https://api.z.ai/api/coding/paas/v4",                                        "Coding/API"},
	{"Codestral",                "https://codestral.mistral.ai/v1",                                            "Coding"},
	{"Qwen Coding Plan",         "https://coding.dashscope.aliyuncs.com/v1",                                   "Coding"},
	{"Azure OpenAI",             "https://{resource}.openai.azure.com/openai/deployments/{deployment}",        "Azure"},
	{"Azure AI Model Inference", "https://{endpoint}.services.ai.azure.com/models",                            "Azure"},
	{"Ollama",                   "http://localhost:11434/v1",                                                   "Local"},
	{"LM Studio",                "http://localhost:1234/v1",                                                    "Local"},
	{"vLLM",                     "http://localhost:8000/v1",                                                    "Local"},
	{"llama.cpp",                "http://localhost:8080/v1",                                                    "Local"},
	{"Jan",                      "http://localhost:1337/v1",                                                    "Local"},
	{"Llamafile",                "http://localhost:8080/v1",                                                    "Local"},
}

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
		// Use a generic default instead of hardcoded OpenAI
		baseURL = providers[0].BaseURL
	}
	// Validate URL is not empty
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
	// Don't set a default API key - user must provide one

	model := os.Getenv("AU_MODEL")
	if model == "" {
		model = s.Model
	}
	if model == "" {
		// Don't set a default model - user should choose
		model = ""
	}

	thinking := s.Thinking
	if thinking < 0 || thinking > 10 {
		thinking = 0
	}

	return Config{BaseURL: s.resolve(baseURL), APIKey: apiKey, Model: model, Thinking: thinking}
}
