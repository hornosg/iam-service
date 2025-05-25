package config

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"iam/src/role/application/usecase"
	"iam/src/role/infrastructure/controller"
	"iam/src/role/infrastructure/persistence/repository"
)

// SetupRoleModule configura e inicializa el módulo de roles
func SetupRoleModule(apiGroup *gin.RouterGroup, db *sql.DB) {
	// Crear repositorio PostgreSQL
	roleRepo := repository.NewPostgresRoleRepository(db)

	// Crear casos de uso
	createRoleUseCase := usecase.NewCreateRoleUseCase(roleRepo)
	getRoleByIDUseCase := usecase.NewGetRoleByIDUseCase(roleRepo)
	updateRoleUseCase := usecase.NewUpdateRoleUseCase(roleRepo)
	deleteRoleUseCase := usecase.NewDeleteRoleUseCase(roleRepo)
	listRolesUseCase := usecase.NewListRolesUseCase(roleRepo)

	// Configurar controlador HTTP
	roleHandler := controller.NewRoleHandler(
		createRoleUseCase,
		getRoleByIDUseCase,
		updateRoleUseCase,
		deleteRoleUseCase,
		listRolesUseCase,
	)

	// Registrar rutas HTTP
	roleHandler.RegisterRoutes(apiGroup)
}
