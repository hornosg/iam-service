package config

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"github.com/hornosg/go-shared/domain/service"
	"github.com/hornosg/iam-service/src/identity/application/usecase"
	"github.com/hornosg/iam-service/src/identity/infrastructure/controller"
	"github.com/hornosg/iam-service/src/identity/infrastructure/persistence/repository"
)

// SetupUserModule configura e inicializa el módulo de usuarios
// Retorna el UserFinderService para que otros módulos puedan usarlo
func SetupUserModule(router *gin.RouterGroup, db *sql.DB) service.UserFinderService {
	// 1. Crear repositorio
	userRepo := repository.NewPostgresUserRepository(db)

	// 2. Crear casos de uso
	createUserUseCase := usecase.NewCreateUserUseCase(userRepo)
	updateUserUseCase := usecase.NewUpdateUserUseCase(userRepo)
	getUserUseCase := usecase.NewGetUserByIDUseCase(userRepo)
	deleteUserUseCase := usecase.NewDeleteUserUseCase(userRepo)
	userFinderUseCase := usecase.NewUserFinderUseCase(userRepo)

	// Crear el nuevo caso de uso de listado por criteria
	listUsersByCriteriaUseCase := usecase.NewListUsersByCriteriaUseCase(userRepo)

	// 3. Crear controlador HTTP
	userHandler := controller.NewUserHandler(
		createUserUseCase,
		getUserUseCase,
		updateUserUseCase,
		deleteUserUseCase,
		listUsersByCriteriaUseCase,
	)

	// 4. Registrar rutas HTTP
	userHandler.RegisterRoutes(router)

	// 5. Retornar servicio para otros módulos
	return userFinderUseCase
}
