package config

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"iam/src/role/application/usecase"
	"iam/src/role/infrastructure/controller"
	"iam/src/role/infrastructure/criteria"
	"iam/src/role/infrastructure/persistence/repository"
)

// SetupRoleModule configura e inicializa el módulo de roles.
//
// ACC-E02 T10: las rutas de lectura (GET /roles, GET /roles/:id) se registran en
// readGroup (legible por autenticado); las de escritura (POST/PUT/DELETE) en
// writeGroup (gated `system:admin`). `roles` es catálogo global sin RLS: el gate
// de scope es la única defensa de la tabla.
func SetupRoleModule(readGroup, writeGroup *gin.RouterGroup, db *sql.DB) {
	// Crear repositorio PostgreSQL
	roleRepo := repository.NewPostgresRoleRepository(db)

	// Crear casos de uso
	createRoleUseCase := usecase.NewCreateRoleUseCase(roleRepo)
	getRoleByIDUseCase := usecase.NewGetRoleByIDUseCase(roleRepo)
	updateRoleUseCase := usecase.NewUpdateRoleUseCase(roleRepo)
	deleteRoleUseCase := usecase.NewDeleteRoleUseCase(roleRepo)
	listRolesUseCase := usecase.NewListRolesUseCase(roleRepo)
	listRolesByCriteriaUseCase := usecase.NewListRolesByCriteriaUseCase(roleRepo)

	// Crear criteria builder
	criteriaBuilder := criteria.NewRoleCriteriaBuilder()

	// Configurar controlador HTTP
	roleHandler := controller.NewRoleHandler(
		createRoleUseCase,
		getRoleByIDUseCase,
		updateRoleUseCase,
		deleteRoleUseCase,
		listRolesUseCase,
		listRolesByCriteriaUseCase,
		criteriaBuilder,
	)

	// Registrar rutas HTTP (lectura en readGroup, escritura en writeGroup)
	roleHandler.RegisterRoutes(readGroup, writeGroup)
}
