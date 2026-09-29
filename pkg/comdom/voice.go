package comdom

// CloneMode — режим клонирования голоса.
type CloneMode string

const (
	// CloneModeInstant — мгновенное клонирование (IVC): голос готов сразу.
	CloneModeInstant CloneMode = "instant"
	// CloneModeProfessional — профессиональное клонирование (PVC): асинхронное обучение.
	CloneModeProfessional CloneMode = "professional"
)

func (m CloneMode) IsValid() bool {
	return m == CloneModeInstant || m == CloneModeProfessional
}

// Voice is provider-neutral metadata for a preset or custom voice.
type Voice struct {
	ID              string   `json:"id"`
	Name            string   `json:"name,omitempty"`
	Age             *int     `json:"age,omitempty"`
	Color           *string  `json:"color,omitempty"`
	CreatedAt       string   `json:"created_at,omitempty"`
	Description     *string  `json:"description,omitempty"`
	Gender          *string  `json:"gender,omitempty"`
	Languages       []string `json:"languages,omitempty"`
	RetentionNotice int      `json:"retention_notice,omitempty"`
	Slug            *string  `json:"slug,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	TrimmedSeconds  *float64 `json:"trimmed_seconds,omitempty"`
	UserID          *string  `json:"user_id,omitempty"`

	// Fine-tuning статус (актуально для PVC).
	FineTuningState    string `json:"fine_tuning_state,omitempty"`    // not_started|queued|fine_tuning|fine_tuned|failed
	FineTuningProgress *int   `json:"fine_tuning_progress,omitempty"` // 0..100
	CloneMode          string `json:"clone_mode,omitempty"`           // instant|professional
}

type VoiceList struct {
	Items      []Voice `json:"items"`
	Page       int     `json:"page"`
	PageSize   int     `json:"page_size"`
	Total      int     `json:"total"`
	TotalPages int     `json:"total_pages"`
}

type CreateVoiceRequest struct {
	Name            string   `json:"name"`
	SampleAudio     string   `json:"sample_audio"`      // base64 (IVC: один образец)
	Samples         []string `json:"samples,omitempty"` // base64, несколько образцов (PVC)
	SampleFilename  *string  `json:"sample_filename,omitempty"`
	CloneMode       string   `json:"clone_mode,omitempty"` // instant|professional, по умолчанию instant
	Language        string   `json:"language,omitempty"`   // обязателен для PVC
	ModelID         string   `json:"model_id,omitempty"`   // опционально для PVC train
	Age             *int     `json:"age,omitempty"`
	Color           *string  `json:"color,omitempty"`
	Description     *string  `json:"description,omitempty"`
	Gender          *string  `json:"gender,omitempty"`
	Languages       []string `json:"languages,omitempty"`
	RetentionNotice int      `json:"retention_notice,omitempty"`
	Slug            *string  `json:"slug,omitempty"`
	Tags            []string `json:"tags,omitempty"`
}

type UpdateVoiceRequest struct {
	Name        *string  `json:"name,omitempty"`
	Age         *int     `json:"age,omitempty"`
	Description *string  `json:"description,omitempty"`
	Gender      *string  `json:"gender,omitempty"`
	Languages   []string `json:"languages,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}
