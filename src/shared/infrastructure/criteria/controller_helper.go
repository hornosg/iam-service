package criteria

import (
	"net/url"

	domainCriteria "iam/src/shared/domain/criteria"

	"github.com/gin-gonic/gin"
)

// ControllerHelper proporciona funciones base para trabajar con criterios en controllers
type ControllerHelper struct{}

// NewControllerHelper crea una nueva instancia del helper
func NewControllerHelper() *ControllerHelper {
	return &ControllerHelper{}
}

// BuildCriteriaFromQuery construye criterios base desde query parameters de Gin
func (h *ControllerHelper) BuildCriteriaFromQuery(c *gin.Context) *domainCriteria.CriteriaBuilder {
	return domainCriteria.NewCriteriaBuilder().FromURLValues(c.Request.URL.Query())
}

// BuildCriteriaFromURLValues construye criterios base desde url.Values
func (h *ControllerHelper) BuildCriteriaFromURLValues(values url.Values) *domainCriteria.CriteriaBuilder {
	return domainCriteria.NewCriteriaBuilder().FromURLValues(values)
}

// ValidateAndSanitizeCriteria valida y sanitiza criterios antes de usarlos
func (h *ControllerHelper) ValidateAndSanitizeCriteria(criteria domainCriteria.Criteria, allowedFields []string) domainCriteria.Criteria {
	if len(allowedFields) == 0 {
		return criteria
	}

	// Crear un mapa para búsqueda rápida
	allowedMap := make(map[string]bool)
	for _, field := range allowedFields {
		allowedMap[field] = true
	}

	// Filtrar solo campos permitidos
	validFilters := domainCriteria.NewFilters()
	for _, filter := range criteria.Filters.Items {
		if allowedMap[filter.Field] {
			validFilters.Add(filter)
		}
	}

	// Validar campo de ordenamiento
	validOrder := criteria.Order
	if validOrder.Field != "" && !allowedMap[validOrder.Field] {
		validOrder = domainCriteria.NewOrder("created_at", "DESC")
	}

	return domainCriteria.NewCriteria(validFilters, validOrder, criteria.Pagination)
}

// BaseCriteriaBuilder interface que deben implementar los builders específicos de cada módulo
type BaseCriteriaBuilder interface {
	// Build construye los criterios finales
	Build() domainCriteria.Criteria

	// GetAllowedFields retorna los campos permitidos para filtrado
	GetAllowedFields() []string
}

// EntityCriteriaHelper helper base que pueden usar los módulos para construir sus builders
type EntityCriteriaHelper struct {
	*ControllerHelper
}

// NewEntityCriteriaHelper crea un nuevo helper para entidades
func NewEntityCriteriaHelper() *EntityCriteriaHelper {
	return &EntityCriteriaHelper{
		ControllerHelper: NewControllerHelper(),
	}
}

// BuildBaseFromContext crea un builder base desde el contexto de Gin
func (h *EntityCriteriaHelper) BuildBaseFromContext(c *gin.Context) *domainCriteria.CriteriaBuilder {
	return h.BuildCriteriaFromQuery(c)
}
