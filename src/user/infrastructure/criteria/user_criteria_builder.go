package criteria

import (
	"github.com/gin-gonic/gin"

	"iam/src/shared/domain/criteria"
	sharedCriteria "iam/src/shared/infrastructure/criteria"
)

// UserCriteriaBuilder construye criterios específicos para usuarios
type UserCriteriaBuilder struct {
	helper  *sharedCriteria.EntityCriteriaHelper
	builder *criteria.CriteriaBuilder
}

// NewUserCriteriaBuilder crea un nuevo builder para criterios de usuarios
func NewUserCriteriaBuilder() *UserCriteriaBuilder {
	return &UserCriteriaBuilder{
		helper: sharedCriteria.NewEntityCriteriaHelper(),
	}
}

// FromContext construye criterios desde el contexto de Gin
func (b *UserCriteriaBuilder) FromContext(c *gin.Context) *UserCriteriaBuilder {
	b.builder = b.helper.BuildBaseFromContext(c)

	// Filtros específicos de usuarios
	b.builder.AddUUIDFilter("tenant_id", c.Query("tenant_id"))
	b.builder.AddEqualFilter("status", c.Query("status"))
	b.builder.AddUUIDFilter("role_id", c.Query("role_id"))
	b.builder.AddLikeFilter("email", c.Query("email"))
	b.builder.AddLikeFilter("first_name", c.Query("first_name"))
	b.builder.AddLikeFilter("last_name", c.Query("last_name"))
	b.builder.AddEqualFilter("provider", c.Query("provider"))

	return b
}

// Build construye los criterios finales
func (b *UserCriteriaBuilder) Build() criteria.Criteria {
	if b.builder == nil {
		// Si no se ha inicializado desde contexto, crear builder vacío
		b.builder = criteria.NewCriteriaBuilder()
	}
	return b.builder.Build()
}

// GetAllowedFields retorna los campos permitidos para filtrado de usuarios
func (b *UserCriteriaBuilder) GetAllowedFields() []string {
	return []string{
		"id", "email", "first_name", "last_name", "tenant_id",
		"role_id", "status", "provider", "created_at", "updated_at",
	}
}

// BuildValidated construye criterios validados desde el contexto
func (b *UserCriteriaBuilder) BuildValidated(c *gin.Context) criteria.Criteria {
	searchCriteria := b.FromContext(c).Build()
	return b.helper.ValidateAndSanitizeCriteria(searchCriteria, b.GetAllowedFields())
}
