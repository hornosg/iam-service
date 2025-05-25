package value_object

import (
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
