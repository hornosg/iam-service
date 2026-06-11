package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"io"

	"iam/src/auth/application/usecase"
	sharedlog "github.com/hornosg/go-shared/infrastructure/logging"
	authEntity "iam/test/auth/domain/entity"
	"iam/test/auth/infrastructure/persistence/repository"
)

func TestRevokeAllUseCase_Execute_HappyPath_RevokesAllTokens(t *testing.T) {
	// Arrange
	mockAuthRepo := repository.NewMockAuthRepository()
	accessTokenExpiry := 15 * time.Minute

	revokeAllUseCase := usecase.NewRevokeAllUseCase(mockAuthRepo, accessTokenExpiry, sharedlog.NewSecurityLoggerWithWriter("iam-test", io.Discard))

	userID := uuid.New()
	tokenMother := authEntity.Create()
	tokens := tokenMother.ForUser(userID, 3)
	mockAuthRepo.SetupRefreshTokens(tokens)

	assert.Equal(t, 3, mockAuthRepo.GetTokenCountByUser(userID))

	// Act
	err := revokeAllUseCase.Execute(context.Background(), userID)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 1, mockAuthRepo.GetCallCount("RevokeAllUserTokens"))
	assert.Equal(t, 1, mockAuthRepo.GetCallCount("DeleteAllUserRefreshTokens"))
	assert.Equal(t, 0, mockAuthRepo.GetTokenCountByUser(userID))
}

func TestRevokeAllUseCase_Execute_RevokeAllFails_ReturnsError(t *testing.T) {
	// Arrange
	mockAuthRepo := repository.NewMockAuthRepository()
	accessTokenExpiry := 15 * time.Minute

	revokeAllUseCase := usecase.NewRevokeAllUseCase(mockAuthRepo, accessTokenExpiry, sharedlog.NewSecurityLoggerWithWriter("iam-test", io.Discard))
	mockAuthRepo.ShouldFailOn("RevokeAllUserTokens")

	userID := uuid.New()

	// Act
	err := revokeAllUseCase.Execute(context.Background(), userID)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, repository.ErrMockFailedOp, err)
	assert.Equal(t, 1, mockAuthRepo.GetCallCount("RevokeAllUserTokens"))
	assert.Equal(t, 0, mockAuthRepo.GetCallCount("DeleteAllUserRefreshTokens"))
}

func TestRevokeAllUseCase_Execute_DeleteRefreshFails_ReturnsError(t *testing.T) {
	// Arrange
	mockAuthRepo := repository.NewMockAuthRepository()
	accessTokenExpiry := 15 * time.Minute

	revokeAllUseCase := usecase.NewRevokeAllUseCase(mockAuthRepo, accessTokenExpiry, sharedlog.NewSecurityLoggerWithWriter("iam-test", io.Discard))
	mockAuthRepo.ShouldFailOn("DeleteAllUserRefreshTokens")

	userID := uuid.New()

	// Act
	err := revokeAllUseCase.Execute(context.Background(), userID)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, repository.ErrMockFailedOp, err)
	assert.Equal(t, 1, mockAuthRepo.GetCallCount("RevokeAllUserTokens"))
	assert.Equal(t, 1, mockAuthRepo.GetCallCount("DeleteAllUserRefreshTokens"))
}

func TestRevokeAllUseCase_Execute_NoTokens_StillSucceeds(t *testing.T) {
	// Arrange
	mockAuthRepo := repository.NewMockAuthRepository()
	accessTokenExpiry := 15 * time.Minute

	revokeAllUseCase := usecase.NewRevokeAllUseCase(mockAuthRepo, accessTokenExpiry, sharedlog.NewSecurityLoggerWithWriter("iam-test", io.Discard))
	userID := uuid.New()

	// Act
	err := revokeAllUseCase.Execute(context.Background(), userID)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 1, mockAuthRepo.GetCallCount("RevokeAllUserTokens"))
	assert.Equal(t, 1, mockAuthRepo.GetCallCount("DeleteAllUserRefreshTokens"))
}
