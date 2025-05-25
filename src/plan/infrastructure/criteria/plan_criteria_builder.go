package criteria

import (
	"github.com/gin-gonic/gin"

	"iam/src/shared/domain/criteria"
	sharedCriteria "iam/src/shared/infrastructure/criteria"
)

// PlanCriteriaBuilder construye criterios específicos para planes
type PlanCriteriaBuilder struct {
	helper  *sharedCriteria.EntityCriteriaHelper
	builder *criteria.CriteriaBuilder
}

// NewPlanCriteriaBuilder crea un nuevo builder para criterios de planes
func NewPlanCriteriaBuilder() *PlanCriteriaBuilder {
	return &PlanCriteriaBuilder{
		helper: sharedCriteria.NewEntityCriteriaHelper(),
	}
}

// FromContext construye criterios desde el contexto de Gin
func (b *PlanCriteriaBuilder) FromContext(c *gin.Context) *PlanCriteriaBuilder {
	b.builder = b.helper.BuildBaseFromContext(c)

	// Filtros específicos de planes
	b.builder.AddEqualFilter("type", c.Query("type"))
	b.builder.AddEqualFilter("status", c.Query("status"))
	b.builder.AddLikeFilter("name", c.Query("name"))
	b.builder.AddEqualFilter("currency", c.Query("currency"))

	// Filtros especiales
	if c.Query("active") == "true" {
		b.builder.AddEqualFilter("status", "ACTIVE")
	}

	// Filtros de rango para precio
	if minPrice := c.Query("min_price"); minPrice != "" {
		b.builder.AddFilter("price", criteria.OpGreaterThanOrEqual, minPrice)
	}

	if maxPrice := c.Query("max_price"); maxPrice != "" {
		b.builder.AddFilter("price", criteria.OpLessThanOrEqual, maxPrice)
	}

	return b
}

// Build construye los criterios finales
func (b *PlanCriteriaBuilder) Build() criteria.Criteria {
	if b.builder == nil {
		// Si no se ha inicializado desde contexto, crear builder vacío
		b.builder = criteria.NewCriteriaBuilder()
	}
	return b.builder.Build()
}

// GetAllowedFields retorna los campos permitidos para filtrado de planes
func (b *PlanCriteriaBuilder) GetAllowedFields() []string {
	return []string{
		"id", "name", "description", "type", "price", "currency",
		"status", "created_at", "updated_at",
	}
}

// BuildValidated construye criterios validados desde el contexto
func (b *PlanCriteriaBuilder) BuildValidated(c *gin.Context) criteria.Criteria {
	searchCriteria := b.FromContext(c).Build()
	return b.helper.ValidateAndSanitizeCriteria(searchCriteria, b.GetAllowedFields())
}
