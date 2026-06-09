package config

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"github.com/mercadocercano/go-shared/domain/service"
	"iam/src/user/application/usecase"
	"iam/src/user/infrastructure/controller"
	"iam/src/user/infrastructure/persistence/repository"
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

	// 3. Crear controlador HTTP refactorizado (que usa criteria)
	userHandler := controller.NewRefactoredUserHandler(
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
