package mode

const (
	IdleDuration                        = 5 // длительность простоя для закрытия SSE
	IdleOperator                        = 5 // длительность простоя для закрытия оператора
	ErrorTimeOutDurationForAssistAnswer = 3 // Если в сообщении есть файлы они могут долго обрабатываться
	// BatchSize Endpoint
	BatchSize         = 100
	TimePeriodicFlush = 60
	// Retry settings
	RetryMaxAttempts = 3 // Максимальное количество повторных попыток
	RetryBaseDelay   = 1 // Базовая задержка между попытками в секундах
	// Mistral API settings
	MistralBaseURL          = "https://api.mistral.ai/v1"
	MistralAgentsBaseURL    = MistralBaseURL + "/agents"
	MistralAgentsURL        = MistralAgentsBaseURL + "/completions"
	MistralConversationsURL = MistralBaseURL + "/conversations"
	// Google API settings
	GoogleAgentsURL = "https://generativelanguage.googleapis.com/v1beta"
	// OpenAI API settings
	OpenAIAgentsURL = "https://api.openai.com/v1"
)
