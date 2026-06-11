package criteria

import (
	"net/url"

	"github.com/gin-gonic/gin"
	crit "github.com/hornosg/go-shared/criteria"
)

// UserCriteriaBuilder construye criterios específicos para usuarios
type UserCriteriaBuilder struct {
	*crit.CriteriaBuilder
	helper *crit.EntityCriteriaHelper
}

// NewUserCriteriaBuilder crea un nuevo builder para criterios de usuarios
func NewUserCriteriaBuilder() *UserCriteriaBuilder {
	return &UserCriteriaBuilder{
		CriteriaBuilder: crit.NewCriteriaBuilder(),
		helper:          crit.NewEntityCriteriaHelper(),
	}
}

// BuildFromContext construye criterios desde el contexto de Gin con filtro automático de tenant
func (b *UserCriteriaBuilder) BuildFromContext(ctx *gin.Context) *UserCriteriaBuilder {
	// Obtener tenant_id del header X-Tenant-ID y agregarlo automáticamente
	tenantID := ctx.GetHeader("X-Tenant-ID")
	if tenantID != "" {
		// Agregar tenant_id a los query parameters para el filtrado
		query := ctx.Request.URL.Query()
		query.Set("tenant_id", tenantID)
		ctx.Request.URL.RawQuery = query.Encode()
	}

	// Usar el helper para construir criterios base
	b.CriteriaBuilder = b.helper.BuildBaseFromContext(ctx)

	// Añadir filtros específicos de usuarios
	b.AddTenantIDFilter(ctx.Query("tenant_id"))
	b.AddEmailFilter(ctx.Query("email"))
	b.AddFirstNameFilter(ctx.Query("first_name"))
	b.AddLastNameFilter(ctx.Query("last_name"))
	b.AddStatusFilter(ctx.Query("status"))
	b.AddRoleIDFilter(ctx.Query("role_id"))
	b.AddProviderFilter(ctx.Query("provider"))

	return b
}

// BuildValidated construye criterios validados desde el contexto
func (b *UserCriteriaBuilder) BuildValidated(ctx *gin.Context) crit.Criteria {
	// Obtener tenant_id del header X-Tenant-ID y agregarlo automáticamente
	tenantID := ctx.GetHeader("X-Tenant-ID")
	if tenantID != "" {
		// Agregar tenant_id a los query parameters para el filtrado
		query := ctx.Request.URL.Query()
		query.Set("tenant_id", tenantID)
		ctx.Request.URL.RawQuery = query.Encode()
	}

	searchCriteria := b.BuildFromContext(ctx).Build()
	return b.helper.ValidateAndSanitizeCriteria(searchCriteria, b.GetAllowedFields())
}

// FromURLValues inicializa el builder desde url.Values
func (b *UserCriteriaBuilder) FromURLValues(values url.Values) *UserCriteriaBuilder {
	// Construir criterios base
	b.CriteriaBuilder = b.CriteriaBuilder.FromURLValues(values)

	// Filtros específicos de usuarios
	b.AddTenantIDFilter(values.Get("tenant_id"))
	b.AddEmailFilter(values.Get("email"))
	b.AddFirstNameFilter(values.Get("first_name"))
	b.AddLastNameFilter(values.Get("last_name"))
	b.AddStatusFilter(values.Get("status"))
	b.AddRoleIDFilter(values.Get("role_id"))
	b.AddProviderFilter(values.Get("provider"))

	return b
}

// Métodos específicos para filtros de usuarios

// AddTenantIDFilter añade filtro por tenant_id
func (b *UserCriteriaBuilder) AddTenantIDFilter(tenantID string) *UserCriteriaBuilder {
	if tenantID != "" {
		b.CriteriaBuilder.AddUUIDFilter("tenant_id", tenantID)
	}
	return b
}

// AddEmailFilter añade filtro por email (búsqueda parcial)
func (b *UserCriteriaBuilder) AddEmailFilter(email string) *UserCriteriaBuilder {
	if email != "" {
		b.CriteriaBuilder.AddLikeFilter("email", email)
	}
	return b
}

// AddFirstNameFilter añade filtro por first_name (búsqueda parcial)
func (b *UserCriteriaBuilder) AddFirstNameFilter(firstName string) *UserCriteriaBuilder {
	if firstName != "" {
		b.CriteriaBuilder.AddLikeFilter("first_name", firstName)
	}
	return b
}

// AddLastNameFilter añade filtro por last_name (búsqueda parcial)
func (b *UserCriteriaBuilder) AddLastNameFilter(lastName string) *UserCriteriaBuilder {
	if lastName != "" {
		b.CriteriaBuilder.AddLikeFilter("last_name", lastName)
	}
	return b
}

// AddStatusFilter añade filtro por status
func (b *UserCriteriaBuilder) AddStatusFilter(status string) *UserCriteriaBuilder {
	if status != "" {
		b.CriteriaBuilder.AddEqualFilter("status", status)
	}
	return b
}

// AddRoleIDFilter añade filtro por role_id
func (b *UserCriteriaBuilder) AddRoleIDFilter(roleID string) *UserCriteriaBuilder {
	if roleID != "" {
		b.CriteriaBuilder.AddUUIDFilter("role_id", roleID)
	}
	return b
}

// AddProviderFilter añade filtro por provider
func (b *UserCriteriaBuilder) AddProviderFilter(provider string) *UserCriteriaBuilder {
	if provider != "" {
		b.CriteriaBuilder.AddEqualFilter("provider", provider)
	}
	return b
}

// GetAllowedFields retorna los campos permitidos para filtrado de usuarios
func (b *UserCriteriaBuilder) GetAllowedFields() []string {
	return []string{
		"id", "email", "first_name", "last_name", "tenant_id",
		"role_id", "status", "provider", "created_at", "updated_at",
	}
}
