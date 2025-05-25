package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"iam/src/auth/application/usecase"
	authEntity "iam/test/auth/domain/entity"
	"iam/test/auth/infrastructure/persistence/repository"
)

func TestLogoutUseCase_Execute(t *testing.T) {
	// Arrange
	mockRepo := repository.NewMockAuthRepository()
	logoutUseCase := usecase.NewLogoutUseCase(mockRepo)
	ctx := context.Background()
	tokenMother := authEntity.Create()

	t.Run("debería eliminar todos los refresh tokens del usuario con éxito", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		userID := uuid.New()

		// Crear múltiples refresh tokens para el usuario
		tokens := tokenMother.ForUser(userID, 3)
		mockRepo.SetupRefreshTokens(tokens)

		// Verificar que el usuario tiene tokens antes del logout
		assert.Equal(t, 3, mockRepo.GetTokenCountByUser(userID))

		// Act
		err := logoutUseCase.Execute(ctx, userID)

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("DeleteAllUserRefreshTokens"))

		// Verificar que todos los tokens del usuario fueron eliminados
		assert.Equal(t, 0, mockRepo.GetTokenCountByUser(userID))
	})

	t.Run("debería funcionar aunque el usuario no tenga tokens", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		userID := uuid.New()
		// No configurar tokens para este usuario

		// Act
		err := logoutUseCase.Execute(ctx, userID)

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("DeleteAllUserRefreshTokens"))
	})

	t.Run("debería fallar si DeleteAllUserRefreshTokens falla", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()
		mockRepo.ShouldFailOn("DeleteAllUserRefreshTokens")

		userID := uuid.New()

		// Act
		err := logoutUseCase.Execute(ctx, userID)

		// Assert
		assert.Error(t, err)
		assert.Equal(t, repository.ErrMockFailedOp, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("DeleteAllUserRefreshTokens"))
	})

	t.Run("debería manejar múltiples usuarios independientemente", func(t *testing.T) {
		// Arrange
		mockRepo.ResetFailures()
		mockRepo.ResetCallHistory()

		user1ID := uuid.New()
		user2ID := uuid.New()

		// Crear tokens para ambos usuarios
		tokens1 := tokenMother.ForUser(user1ID, 2)
		tokens2 := tokenMother.ForUser(user2ID, 3)
		allTokens := append(tokens1, tokens2...)
		mockRepo.SetupRefreshTokens(allTokens)

		// Verificar estado inicial
		assert.Equal(t, 2, mockRepo.GetTokenCountByUser(user1ID))
		assert.Equal(t, 3, mockRepo.GetTokenCountByUser(user2ID))

		// Act - Hacer logout solo del primer usuario
		err := logoutUseCase.Execute(ctx, user1ID)

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, 1, mockRepo.GetCallCount("DeleteAllUserRefreshTokens"))

		// Verificar que solo los tokens del primer usuario fueron eliminados
		assert.Equal(t, 0, mockRepo.GetTokenCountByUser(user1ID))
		assert.Equal(t, 3, mockRepo.GetTokenCountByUser(user2ID)) // Tokens del segundo usuario intactos
	})
}
