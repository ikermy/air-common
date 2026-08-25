package comdb

import (
	"database/sql"
	"errors"
	"fmt"
)

// SetContactAvailability сохраняет доступность контакта в конкретном провайдере
func (d *DB) SetContactAvailability(userID uint32, contact, provider string, isAvailable bool) error {
	// Сначала получаем ContactId из service_contacts
	var contactID int64
	query := `SELECT Id FROM service_contacts WHERE userID = ? AND Contact = ? LIMIT 1`
	err := d.Conn().QueryRow(query, userID, contact).Scan(&contactID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("контакт %s не найден в service_contacts для пользователя %d", contact, userID)
		}
		return fmt.Errorf("ошибка получения ContactId: %w", err)
	}

	// Сохраняем доступность через ContactId
	insertQuery := `
		INSERT INTO service_contact_availability 
			(ContactId, Provider, IsAvailable, CheckedAt, UpdatedAt)
		VALUES (?, ?, ?, NOW(), NOW())
		ON DUPLICATE KEY UPDATE 
			IsAvailable = VALUES(IsAvailable),
			UpdatedAt = NOW()
	`

	_, err = d.Conn().Exec(insertQuery, contactID, provider, isAvailable)
	if err != nil {
		return fmt.Errorf("ошибка сохранения доступности контакта: %w", err)
	}

	return nil
}

// GetContactAvailability получает доступность контакта во всех провайдерах
func (d *DB) GetContactAvailability(userID uint32, contact string) (map[string]bool, error) {
	query := `
		SELECT ca.Provider, ca.IsAvailable 
		FROM service_contact_availability ca
		INNER JOIN service_contacts c ON ca.ContactId = c.Id
		WHERE c.userID = ? AND c.Contact = ?
	`

	rows, err := d.Conn().Query(query, userID, contact)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения доступности контакта: %w", err)
	}
	defer rows.Close()

	availability := make(map[string]bool)
	for rows.Next() {
		var provider string
		var isAvailable bool
		if err := rows.Scan(&provider, &isAvailable); err != nil {
			return nil, fmt.Errorf("ошибка чтения данных доступности: %w", err)
		}
		availability[provider] = isAvailable
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ошибка итерации результатов: %w", err)
	}

	return availability, nil
}

// GetContactsAvailableIn получает список контактов доступных в указанном провайдере
func (d *DB) GetContactsAvailableIn(userID uint32, provider string) ([]string, error) {
	query := `
		SELECT DISTINCT c.Contact 
		FROM service_contact_availability ca
		INNER JOIN service_contacts c ON ca.ContactId = c.Id
		WHERE c.userID = ? 
		  AND ca.Provider = ? 
		  AND ca.IsAvailable = 1
		ORDER BY c.Contact
	`

	rows, err := d.Conn().Query(query, userID, provider)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения контактов для провайдера %s: %w", provider, err)
	}
	defer rows.Close()

	var contacts []string
	for rows.Next() {
		var contact string
		if err := rows.Scan(&contact); err != nil {
			return nil, fmt.Errorf("ошибка чтения контакта: %w", err)
		}
		contacts = append(contacts, contact)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ошибка итерации результатов: %w", err)
	}

	return contacts, nil
}

// GetContactsInBothProviders получает контакты доступные в обеих указанных платформах
func (d *DB) GetContactsInBothProviders(userID uint32, provider1, provider2 string) ([]string, error) {
	query := `
		SELECT c.Contact
		FROM service_contacts c
		INNER JOIN service_contact_availability ca1 ON c.Id = ca1.ContactId
		INNER JOIN service_contact_availability ca2 ON c.Id = ca2.ContactId
		WHERE c.userID = ?
		  AND ca1.Provider = ?
		  AND ca1.IsAvailable = 1
		  AND ca2.Provider = ?
		  AND ca2.IsAvailable = 1
		ORDER BY c.Contact
	`

	rows, err := d.Conn().Query(query, userID, provider1, provider2)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения общих контактов: %w", err)
	}
	defer rows.Close()

	var contacts []string
	for rows.Next() {
		var contact string
		if err := rows.Scan(&contact); err != nil {
			return nil, fmt.Errorf("ошибка чтения контакта: %w", err)
		}
		contacts = append(contacts, contact)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ошибка итерации результатов: %w", err)
	}

	return contacts, nil
}
