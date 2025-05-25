package exception

import "errors"

var (
	ErrInvalidStatus     = errors.New("estado de usuario inválido")
	ErrUserNotFound      = errors.New("usuario no encontrado")
	ErrUserAlreadyExists = errors.New("usuario ya existe")
	ErrInvalidEmail      = errors.New("email inválido")
	ErrWeakPassword      = errors.New("contraseña débil")
)
