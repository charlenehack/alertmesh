package model

// LLMProvider is the per-row configuration of an upstream LLM endpoint
// used by the AI agent (root-cause analysis + follow-up chat).
//
// Three behaviour knobs ride alongside the connection details so they can
// be tuned per-provider from the admin UI without redeploying:
//
//   - Language               – output language ("zh" / "en" / "auto").
//   - ChatReportMaxChars     – cap on prior-report context re-fed into chat.
//   - ChatHistoryMaxTurns    – cap on prior-conversation pairs re-fed.
//
// All three may be left at 0 / "" in the DB; agent.go contains process-level
// fallbacks so existing rows seeded before migration 33 keep working.
type LLMProvider struct {
	ID          string  `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Name        string  `gorm:"not null"                                       json:"name"`
	Provider    string  `gorm:"not null"                                       json:"provider"` // openai/azure/ollama/anthropic
	BaseURL     string  `json:"base_url"`
	APIKey      string  `gorm:"not null"                                       json:"-"` // AES-256 encrypted
	ModelName   string  `gorm:"column:model;not null"                          json:"model"`
	Temperature float32 `gorm:"default:0.1"                                    json:"temperature"`
	IsDefault   bool    `gorm:"default:false"                                  json:"is_default"`
	IsEnabled   bool    `gorm:"default:true"                                   json:"is_enabled"`

	// AI behaviour knobs (migration 33).  Defaults match the constants in
	// internal/ai/agent.go so existing rows (where these columns get the
	// SQL DEFAULT) and brand-new rows behave identically.
	Language            string `gorm:"type:varchar(8);not null;default:'zh'" json:"language"` // zh|en|auto
	ChatReportMaxChars  int    `gorm:"not null;default:8000"                 json:"chat_report_max_chars"`
	ChatHistoryMaxTurns int    `gorm:"not null;default:10"                   json:"chat_history_max_turns"`

	Timestamps
}

func (LLMProvider) TableName() string { return "llm_providers" }
