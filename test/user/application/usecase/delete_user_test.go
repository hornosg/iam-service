package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"iam/src/user/application/usecase"
	"iam/src/user/domain/exception"
	"iam/src/user/domain/value_object"
	roleEntity "iam/test/role/domain/entity"
	tenantEntity "iam/test/tenant/domain/entity"
	userEntity "iam/test/user/domain/entity"
	"iam/test/user/infrastructure/persistence/repository"
)

func TestDeleteUserUseCase_Execute(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockUserRepository()
	deleteUseCase := usecase.NewDeleteUserUseCase(mockRepo)
	ctx := context.Background()

	// Object Mothers
	userMother := userEntity.Create()
	tenantMother := tenantEntity.Create()
	roleMother := roleEntity.Create()

	t.Run("debería eliminar un usuario activo con éxito", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		// Crear entidades relacionadas usando Object Mothers
		tenant := tenantMother.WithDefaults()
		role := roleMother.User()
		user := userMother.WithTenant(tenant.ID).WithRole(role.ID).WithStatus(value_object.StatusActive)

		mockRepo.SetupUsers([]*userEntity.User{user})

		// Act
		err := deleteUseCase.Execute(ctx, user.ID)

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Delete"))

		// Verificar que el usuario fue marcado como eliminado
		users := mockRepo.GetUsers()
		assert.Len(t, users, 1)
		assert.Equal(t, value_object.StatusDeleted, users[0].Status)
	})

	t.Run("debería eliminar un usuario pendiente con éxito", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		// Crear un usuario pendiente usando Object Mother
		tenant := tenantMother.Startup()
		role := roleMother.TenantAdminForTenant(tenant.ID)
		user := userMother.WithTenant(tenant.ID).WithRole(role.ID).Pending()

		mockRepo.SetupUsers([]*userEntity.User{user})

		// Act
		err := deleteUseCase.Execute(ctx, user.ID)

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Delete"))
	})

	t.Run("debería eliminar un usuario federado con éxito", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		// Crear un usuario federado usando Object Mother
		tenant := tenantMother.Enterprise()
		role := roleMother.ReadOnly()
		user := userMother.WithTenant(tenant.ID).WithRole(role.ID).WithFederatedProvider("GOOGLE", "google123")

		mockRepo.SetupUsers([]*userEntity.User{user})

		// Act
		err := deleteUseCase.Execute(ctx, user.ID)

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Delete"))
	})

	t.Run("debería fallar si el usuario no existe", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		nonExistentID := uuid.New()

		// Act
		err := deleteUseCase.Execute(ctx, nonExistentID)

		// Assert
		assert.Error(t, err)
		assert.Equal(t, exception.ErrUserNotFound, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 0, mockRepo.GetCallCount("Delete")) // No debe llamar a Delete
	})

	t.Run("debería fallar si GetByID falla", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.ShouldFailOn("GetByID")

		userID := uuid.New()

		// Act
		err := deleteUseCase.Execute(ctx, userID)

		// Assert
		assert.Error(t, err)
		assert.Equal(t, exception.ErrUserNotFound, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 0, mockRepo.GetCallCount("Delete"))
	})

	t.Run("debería fallar si Delete falla", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		// Crear un usuario usando Object Mothers
		tenant := tenantMother.Business()
		role := roleMother.Custom()
		user := userMother.WithTenant(tenant.ID).WithRole(role.ID)

		mockRepo.SetupUsers([]*userEntity.User{user})
		mockRepo.ShouldFailOn("Delete")

		// Act
		err := deleteUseCase.Execute(ctx, user.ID)

		// Assert
		assert.Error(t, err)
		assert.Equal(t, repository.ErrMockFailedOp, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Delete"))
	})

	t.Run("debería manejar múltiples usuarios de diferentes tenants", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		// Crear múltiples tenants y roles usando Object Mothers
		tenant1 := tenantMother.WithName("Tenant 1").Startup()
		tenant2 := tenantMother.WithName("Tenant 2").Enterprise()

		role1 := roleMother.TenantAdminForTenant(tenant1.ID)
		role2 := roleMother.UserForTenant(tenant2.ID)

		user1 := userMother.WithEmail("user1@tenant1.com").WithTenant(tenant1.ID).WithRole(role1.ID)
		user2 := userMother.WithEmail("user2@tenant2.com").WithTenant(tenant2.ID).WithRole(role2.ID)

		mockRepo.SetupUsers([]*userEntity.User{user1, user2})

		// Act - Eliminar el primer usuario
		err := deleteUseCase.Execute(ctx, user1.ID)

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Delete"))

		// Verificar que solo el primer usuario fue eliminado
		users := mockRepo.GetUsers()
		assert.Len(t, users, 2)

		var deletedUser, activeUser *userEntity.User
		for _, u := range users {
			if u.ID == user1.ID {
				deletedUser = u
			} else {
				activeUser = u
			}
		}

		assert.NotNil(t, deletedUser)
		assert.NotNil(t, activeUser)
		assert.Equal(t, value_object.StatusDeleted, deletedUser.Status)
		assert.NotEqual(t, value_object.StatusDeleted, activeUser.Status)
	})

	t.Run("debería manejar usuario con rol de sistema", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		// Crear un usuario con rol de sistema usando Object Mothers
		systemRole := roleMother.SystemAdmin()
		user := userMother.WithRole(systemRole.ID).WithEmail("admin@system.com")

		mockRepo.SetupUsers([]*userEntity.User{user})

		// Act
		err := deleteUseCase.Execute(ctx, user.ID)

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
		assert.Equal(t, 1, mockRepo.GetCallCount("Delete"))
	})
}
