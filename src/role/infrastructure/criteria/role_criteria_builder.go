package criteria

import (
	"github.com/gin-gonic/gin"
	crit "github.com/hornosg/go-shared/criteria"
)

// RoleCriteriaBuilder construye criterios específicos para roles
type RoleCriteriaBuilder struct {
	helper  *crit.EntityCriteriaHelper
	builder *crit.CriteriaBuilder
}

// NewRoleCriteriaBuilder crea un nuevo builder para criterios de roles
func NewRoleCriteriaBuilder() *RoleCriteriaBuilder {
	return &RoleCriteriaBuilder{
		helper: crit.NewEntityCriteriaHelper(),
	}
}

// FromContext construye criterios desde el contexto de Gin
func (b *RoleCriteriaBuilder) FromContext(c *gin.Context) *RoleCriteriaBuilder {
	b.builder = b.helper.BuildBaseFromContext(c)

	// Filtros específicos de roles (tabla usa is_active, no status)
	b.builder.AddUUIDFilter("tenant_id", c.Query("tenant_id"))
	b.builder.AddEqualFilter("type", c.Query("type"))
	b.builder.AddLikeFilter("name", c.Query("name"))

	// Filtros especiales
	if c.Query("system") == "true" {
		b.builder.AddEqualFilter("type", "SYSTEM")
	}

	// Por defecto solo roles activos; active=false muestra todos
	if c.Query("active") != "false" {
		b.builder.AddEqualFilter("is_active", "true")
	}

	return b
}

// Build construye los criterios finales
func (b *RoleCriteriaBuilder) Build() crit.Criteria {
	if b.builder == nil {
		b.builder = crit.NewCriteriaBuilder()
	}
	return b.builder.Build()
}

// GetAllowedFields retorna los campos permitidos para filtrado de roles
func (b *RoleCriteriaBuilder) GetAllowedFields() []string {
	return []string{
		"id", "name", "description", "type", "is_active",
		"tenant_id", "created_at", "updated_at",
	}
}

// BuildValidated construye criterios validados desde el contexto
func (b *RoleCriteriaBuilder) BuildValidated(c *gin.Context) crit.Criteria {
	searchCriteria := b.FromContext(c).Build()
	return b.helper.ValidateAndSanitizeCriteria(searchCriteria, b.GetAllowedFields())
}
