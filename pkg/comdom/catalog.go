package comdom

type DefaultProvidersModels struct {
	GeneralModelID    uint
	GeneralModelName  string
	RealTimeModelID   uint
	RealTimeModelName string
}
type ProviderModelUserChange struct {
	UserID    uint32 `json:"user_id"`
	ModelID   uint64 `json:"model_id"`
	ModelName string `json:"model_name,omitempty"`
}
type ProviderModel struct {
	ID   uint64    `json:"Id"`
	Name string    `json:"Name"`
	Kind VoiceKind `json:"kind,omitempty"` // пусто — LLM-модель; иначе tts|stt|music|sts

	IsDefault            bool     `json:"is_default,omitempty"`
	DisplayName          string   `json:"display_name,omitempty"`
	Languages            []string `json:"languages,omitempty"`
	SupportsStyle        *bool    `json:"supports_style,omitempty"`
	SupportsSpeakerBoost *bool    `json:"supports_speaker_boost,omitempty"`
}

type ProviderModelsSyncResult struct {
	Provider      ProviderType              `json:"provider"`
	Models        []ProviderModel           `json:"models,omitempty"`
	Synced        int                       `json:"synced"`
	Removed       int                       `json:"removed"`
	ClearedUsers  int                       `json:"cleared_users"`
	RemovedNames  []string                  `json:"removed_names,omitempty"`
	AffectedUsers []ProviderModelUserChange `json:"affected_users,omitempty"`
}
