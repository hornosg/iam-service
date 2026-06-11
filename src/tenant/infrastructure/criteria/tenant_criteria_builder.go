package criteria

import (
	"github.com/gin-gonic/gin"
	crit "github.com/hornosg/go-shared/criteria"
)

// TenantCriteriaBuilder construye criterios específicos para tenants
type TenantCriteriaBuilder struct {
	helper  *crit.EntityCriteriaHelper
	builder *crit.CriteriaBuilder
}

// NewTenantCriteriaBuilder crea un nuevo builder para criterios de tenants
func NewTenantCriteriaBuilder() *TenantCriteriaBuilder {
	return &TenantCriteriaBuilder{
		helper: crit.NewEntityCriteriaHelper(),
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
func (b *TenantCriteriaBuilder) Build() crit.Criteria {
	if b.builder == nil {
		b.builder = crit.NewCriteriaBuilder()
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
func (b *TenantCriteriaBuilder) BuildValidated(c *gin.Context) crit.Criteria {
	searchCriteria := b.FromContext(c).Build()
	return b.helper.ValidateAndSanitizeCriteria(searchCriteria, b.GetAllowedFields())
}
