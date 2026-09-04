package entity

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	srcEntity "iam/src/identity/domain/entity"
	"iam/src/identity/domain/exception"
	"iam/src/identity/domain/value_object"
)

func TestNewUser_WithValidParams_CreatesUserWithDefaults(t *testing.T) {
	email, _ := value_object.NewEmail("test@example.com")
	tenantID := uuid.New()
	roleID := uuid.New()

	user := srcEntity.NewUser(email, tenantID, roleID)

	assert.NotEqual(t, uuid.Nil, user.ID)
	assert.Equal(t, "test@example.com", user.Email.Value())
	assert.Equal(t, tenantID, user.TenantID)
	assert.Equal(t, roleID, user.RoleID)
	assert.Equal(t, value_object.StatusPending, user.Status)
	assert.Equal(t, "LOCAL", user.Provider)
	assert.Empty(t, user.PasswordHash)
	assert.False(t, user.CreatedAt.IsZero())
}

func TestUser_SetPassword_HashesPassword(t *testing.T) {
	mother := Create()
	user := mother.WithDefaults()

	err := user.SetPassword("newpassword123")

	require.NoError(t, err)
	assert.NotEmpty(t, user.PasswordHash)
	assert.NotEqual(t, "newpassword123", user.PasswordHash)
}

func TestUser_ValidatePassword_CorrectPassword_Succeeds(t *testing.T) {
	mother := Create()
	user := mother.WithDefaults()
	_ = user.SetPassword("correctpassword")

	err := user.ValidatePassword("correctpassword")

	assert.NoError(t, err)
}

func TestUser_ValidatePassword_WrongPassword_ReturnsError(t *testing.T) {
	mother := Create()
	user := mother.WithDefaults()
	_ = user.SetPassword("correctpassword")

	err := user.ValidatePassword("wrongpassword")

	assert.Error(t, err)
}

func TestUser_ChangeStatus_ValidStatus_ChangesStatus(t *testing.T) {
	mother := Create()
	user := mother.WithDefaults()

	err := user.ChangeStatus(value_object.StatusActive)

	require.NoError(t, err)
	assert.Equal(t, value_object.StatusActive, user.Status)
}

func TestUser_ChangeStatus_InvalidStatus_ReturnsError(t *testing.T) {
	mother := Create()
	user := mother.WithDefaults()

	err := user.ChangeStatus(value_object.UserStatus("INVALID"))

	assert.Error(t, err)
	assert.Equal(t, exception.ErrInvalidStatus, err)
}

func TestUser_IsActive_ActiveUser_ReturnsTrue(t *testing.T) {
	mother := Create()
	user := mother.WithStatus(value_object.StatusActive)

	assert.True(t, user.IsActive())
}

func TestUser_IsActive_InactiveUser_ReturnsFalse(t *testing.T) {
	mother := Create()
	user := mother.WithStatus(value_object.StatusInactive)

	assert.False(t, user.IsActive())
}

func TestUser_IsPending_PendingUser_ReturnsTrue(t *testing.T) {
	mother := Create()
	user := mother.Pending()

	assert.True(t, user.IsPending())
}

func TestUser_IsPending_ActiveUser_ReturnsFalse(t *testing.T) {
	mother := Create()
	user := mother.WithStatus(value_object.StatusActive)

	assert.False(t, user.IsPending())
}

func TestUser_UpdateEmail_ChangesEmail(t *testing.T) {
	mother := Create()
	user := mother.WithDefaults()
	newEmail, _ := value_object.NewEmail("new@example.com")

	user.UpdateEmail(newEmail)

	assert.Equal(t, "new@example.com", user.Email.Value())
}

func TestUser_LinkFederatedID_SetsProviderAndID(t *testing.T) {
	mother := Create()
	user := mother.WithDefaults()

	user.LinkFederatedID("GOOGLE", "google-id-123")

	assert.Equal(t, "GOOGLE", user.Provider)
	assert.Equal(t, "google-id-123", user.FederatedID)
}
