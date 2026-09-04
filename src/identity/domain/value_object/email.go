package value_object

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

type Email struct {
	value string
}

func NewEmail(email string) (*Email, error) {
	email = strings.TrimSpace(email)

	if email == "" {
		return nil, errors.New("email no puede estar vacío")
	}

	if !isValidEmail(email) {
		return nil, errors.New("formato de email inválido")
	}

	return &Email{value: email}, nil
}

func (e Email) Value() string {
	return e.value
}

func (e Email) String() string {
	return e.value
}

func (e Email) Domain() string {
	parts := strings.Split(e.value, "@")
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

func isValidEmail(email string) bool {
	// Regex básica para validación de email
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}

// MarshalJSON implementa la serialización JSON para Email
func (e Email) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.value)
}

// UnmarshalJSON implementa la deserialización JSON para Email
func (e *Email) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}

	email, err := NewEmail(value)
	if err != nil {
		return err
	}

	*e = *email
	return nil
}
