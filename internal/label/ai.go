package label

const (
	AIModuleName = "AI 分析"

	// AI 分析功能随告警事件权限（IncidentAccess），此处仅保留 LLM 供应商配置（管理员专属）
	LLMProviderList    = "llmProviderList"
	LLMProviderCreate  = "llmProviderCreate"
	LLMProviderUpdate  = "llmProviderUpdate"
	LLMProviderDelete  = "llmProviderDelete"
	LLMProviderDefault = "llmProviderDefault"
	LLMProviderTest    = "llmProviderTest"
)
