package port

import (
	"github.com/mercadocercano/go-shared/domain/service"
)

// UserService es un alias de la interfaz compartida para mantener consistencia en el módulo auth
type UserService = service.UserFinderService

// UserData es un alias del tipo compartido
type UserData = service.BasicUserData
