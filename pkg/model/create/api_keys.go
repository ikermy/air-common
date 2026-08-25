package create

import (
	"github.com/ikermy/air-common/pkg/comdom"
)

// ProvidersWithApiKeys проверяет наличие пользовательских ключей у всех
// зарегистрированных провайдеров.
func (m *UniversalModel) ProvidersWithApiKeys(userID uint32) comdom.ProvidersAvailability {
	result := comdom.ProvidersAvailability{Available: make([]string, 0), Unavailable: make([]string, 0)}
	for _, p := range comdom.AllProviders {
		key, err := m.db.GetUserAPIKey(userID, p)
		if err == nil && key != "" {
			result.Available = append(result.Available, p.String())
		} else {
			result.Unavailable = append(result.Unavailable, p.String())
		}
	}
	return result
}

func (m *UniversalModel) SetUserAPIKey(userID uint32, provider comdom.ProviderType, key string) error {
	return m.db.SetUserAPIKey(userID, provider, key)
}
func (m *UniversalModel) GetUserAPIKey(userID uint32, provider comdom.ProviderType) (string, error) {
	return m.db.GetUserAPIKey(userID, provider)
}
func (m *UniversalModel) DeleteUserAPIKey(userID uint32, provider comdom.ProviderType) error {
	return m.db.DeleteUserAPIKey(userID, provider)
}
