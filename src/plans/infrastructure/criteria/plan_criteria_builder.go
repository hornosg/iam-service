package criteria

import (
	"github.com/gin-gonic/gin"
	crit "github.com/hornosg/go-shared/criteria"
)

// PlanCriteriaBuilder construye criterios específicos para planes
type PlanCriteriaBuilder struct {
	helper  *crit.EntityCriteriaHelper
	builder *crit.CriteriaBuilder
}

// NewPlanCriteriaBuilder crea un nuevo builder para criterios de planes
func NewPlanCriteriaBuilder() *PlanCriteriaBuilder {
	return &PlanCriteriaBuilder{
		helper: crit.NewEntityCriteriaHelper(),
	}
}

// FromContext construye criterios desde el contexto de Gin
func (b *PlanCriteriaBuilder) FromContext(c *gin.Context) *PlanCriteriaBuilder {
	b.builder = b.helper.BuildBaseFromContext(c)

	// Filtros específicos de planes
	b.builder.AddEqualFilter("type", c.Query("type"))
	b.builder.AddEqualFilter("status", c.Query("status"))
	b.builder.AddLikeFilter("name", c.Query("name"))
	// Filtros especiales
	if c.Query("active") == "true" {
		b.builder.AddEqualFilter("status", "ACTIVE")
	}

	// Filtros de rango para precio (price_month)
	if minPrice := c.Query("min_price"); minPrice != "" {
		b.builder.AddFilter("price_month", crit.OpGreaterThanOrEqual, minPrice)
	}

	if maxPrice := c.Query("max_price"); maxPrice != "" {
		b.builder.AddFilter("price_month", crit.OpLessThanOrEqual, maxPrice)
	}

	return b
}

// Build construye los criterios finales
func (b *PlanCriteriaBuilder) Build() crit.Criteria {
	if b.builder == nil {
		b.builder = crit.NewCriteriaBuilder()
	}
	return b.builder.Build()
}

// GetAllowedFields retorna los campos permitidos para filtrado de planes
func (b *PlanCriteriaBuilder) GetAllowedFields() []string {
	return []string{
		"id", "name", "description", "type", "price_month", "price_year",
		"status", "created_at", "updated_at",
	}
}

// BuildValidated construye criterios validados desde el contexto
func (b *PlanCriteriaBuilder) BuildValidated(c *gin.Context) crit.Criteria {
	searchCriteria := b.FromContext(c).Build()
	return b.helper.ValidateAndSanitizeCriteria(searchCriteria, b.GetAllowedFields())
}
