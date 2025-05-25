package criteria

import (
	"github.com/gin-gonic/gin"

	"iam/src/shared/domain/criteria"
	sharedCriteria "iam/src/shared/infrastructure/criteria"
)

// TenantCriteriaBuilder construye criterios específicos para tenants
type TenantCriteriaBuilder struct {
	helper  *sharedCriteria.EntityCriteriaHelper
	builder *criteria.CriteriaBuilder
}

// NewTenantCriteriaBuilder crea un nuevo builder para criterios de tenants
func NewTenantCriteriaBuilder() *TenantCriteriaBuilder {
	return &TenantCriteriaBuilder{
		helper: sharedCriteria.NewEntityCriteriaHelper(),
	}
}

// FromContext construye criterios desde el contexto de Gin
func (b *TenantCriteriaBuilder) FromContext(c *gin.Context) *TenantCriteriaBuilder {
	b.builder = b.helper.BuildBaseFromContext(c)

	// Filtros específicos de tenants
	b.builder.AddUUIDFilter("owner_id", c.Query("owner_id"))
	b.builder.AddEqualFilter("status", c.Query("status"))
	b.builder.AddEqualFilter("type", c.Query("type"))
	b.builder.AddUUIDFilter("plan_id", c.Query("plan_id"))
	b.builder.AddLikeFilter("name", c.Query("name"))
	b.builder.AddLikeFilter("slug", c.Query("slug"))
	b.builder.AddLikeFilter("domain", c.Query("domain"))

	// Filtros especiales
	if c.Query("active") == "true" {
		b.builder.AddEqualFilter("status", "ACTIVE")
	}

	return b
}

// Build construye los criterios finales
func (b *TenantCriteriaBuilder) Build() criteria.Criteria {
	if b.builder == nil {
		// Si no se ha inicializado desde contexto, crear builder vacío
		b.builder = criteria.NewCriteriaBuilder()
	}
	return b.builder.Build()
}

// GetAllowedFields retorna los campos permitidos para filtrado de tenants
func (b *TenantCriteriaBuilder) GetAllowedFields() []string {
	return []string{
		"id", "name", "slug", "domain", "type", "status",
		"owner_id", "plan_id", "created_at", "updated_at",
	}
}

// BuildValidated construye criterios validados desde el contexto
func (b *TenantCriteriaBuilder) BuildValidated(c *gin.Context) criteria.Criteria {
	searchCriteria := b.FromContext(c).Build()
	return b.helper.ValidateAndSanitizeCriteria(searchCriteria, b.GetAllowedFields())
}
