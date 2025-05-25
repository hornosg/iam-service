package criteria

import (
	"net/url"
	"strconv"
	"strings"
)

// BaseListRequest representa los parámetros básicos de cualquier endpoint de lista
type BaseListRequest struct {
	Page     int    `json:"page" form:"page"`
	PageSize int    `json:"page_size" form:"page_size"`
	SortBy   string `json:"sort_by" form:"sort_by"`
	SortDir  string `json:"sort_dir" form:"sort_dir"`
}

// Validate valida y normaliza los parámetros básicos
func (r *BaseListRequest) Validate() {
	if r.Page < 1 {
		r.Page = 1
	}
	if r.PageSize < 1 {
		r.PageSize = 10
	}
	if r.PageSize > 100 {
		r.PageSize = 100
	}
	if r.SortDir != "asc" && r.SortDir != "desc" {
		r.SortDir = "desc"
	}
	if r.SortBy == "" {
		r.SortBy = "created_at"
	}
}

// GetPagination retorna un objeto Pagination basado en los parámetros
func (r *BaseListRequest) GetPagination() Pagination {
	r.Validate()
	return NewPaginationFromPage(r.Page, r.PageSize)
}

// GetOrder retorna un objeto Order basado en los parámetros
func (r *BaseListRequest) GetOrder() Order {
	r.Validate()
	return NewOrder(r.SortBy, strings.ToUpper(r.SortDir))
}

// GetBaseCriteria retorna un Criteria con paginación y orden básicos
func (r *BaseListRequest) GetBaseCriteria() Criteria {
	return NewCriteria(NewFilters(), r.GetOrder(), r.GetPagination())
}

// FilterParam representa un parámetro de filtro individual
type FilterParam struct {
	Field    string
	Value    string
	Operator string
}

// CriteriaBuilder ayuda a construir objetos Criteria desde query parameters
type CriteriaBuilder struct {
	filters     Filters
	baseRequest BaseListRequest
}

// NewCriteriaBuilder crea un nuevo builder
func NewCriteriaBuilder() *CriteriaBuilder {
	return &CriteriaBuilder{
		filters: NewFilters(),
		baseRequest: BaseListRequest{
			Page:     1,
			PageSize: 10,
			SortBy:   "created_at",
			SortDir:  "desc",
		},
	}
}

// FromURLValues construye criterios desde valores de URL
func (cb *CriteriaBuilder) FromURLValues(values url.Values) *CriteriaBuilder {
	// Parsear parámetros básicos
	if page := values.Get("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil {
			cb.baseRequest.Page = p
		}
	}

	if pageSize := values.Get("page_size"); pageSize != "" {
		if ps, err := strconv.Atoi(pageSize); err == nil {
			cb.baseRequest.PageSize = ps
		}
	}

	if sortBy := values.Get("sort_by"); sortBy != "" {
		cb.baseRequest.SortBy = sortBy
	}

	if sortDir := values.Get("sort_dir"); sortDir != "" {
		cb.baseRequest.SortDir = sortDir
	}

	return cb
}

// AddFilter agrega un filtro si el valor no está vacío
func (cb *CriteriaBuilder) AddFilter(field, operator, value string) *CriteriaBuilder {
	if value != "" {
		cb.filters.AddFilter(field, operator, value)
	}
	return cb
}

// AddEqualFilter agrega un filtro de igualdad
func (cb *CriteriaBuilder) AddEqualFilter(field, value string) *CriteriaBuilder {
	return cb.AddFilter(field, "=", value)
}

// AddLikeFilter agrega un filtro LIKE
func (cb *CriteriaBuilder) AddLikeFilter(field, value string) *CriteriaBuilder {
	return cb.AddFilter(field, "LIKE", value)
}

// AddBoolFilter agrega un filtro booleano
func (cb *CriteriaBuilder) AddBoolFilter(field string, value bool) *CriteriaBuilder {
	cb.filters.AddFilter(field, "=", value)
	return cb
}

// AddInFilter agrega un filtro IN para arrays
func (cb *CriteriaBuilder) AddInFilter(field string, values []string) *CriteriaBuilder {
	if len(values) > 0 {
		cb.filters.AddFilter(field, "IN", values)
	}
	return cb
}

// AddUUIDFilter agrega un filtro para UUID si es válido
func (cb *CriteriaBuilder) AddUUIDFilter(field, value string) *CriteriaBuilder {
	if value != "" && isValidUUIDFormat(value) {
		cb.filters.AddFilter(field, "=", value)
	}
	return cb
}

// Build construye el objeto Criteria final
func (cb *CriteriaBuilder) Build() Criteria {
	return NewCriteria(cb.filters, cb.baseRequest.GetOrder(), cb.baseRequest.GetPagination())
}

// isValidUUIDFormat verifica si una cadena tiene formato de UUID válido
func isValidUUIDFormat(uuid string) bool {
	// Regex simple para UUID v4
	if len(uuid) != 36 {
		return false
	}

	parts := strings.Split(uuid, "-")
	if len(parts) != 5 {
		return false
	}

	expectedLengths := []int{8, 4, 4, 4, 12}
	for i, part := range parts {
		if len(part) != expectedLengths[i] {
			return false
		}
	}

	return true
}

// Common filter operators
const (
	OpEqual              = "="
	OpNotEqual           = "!="
	OpGreaterThan        = ">"
	OpGreaterThanOrEqual = ">="
	OpLessThan           = "<"
	OpLessThanOrEqual    = "<="
	OpLike               = "LIKE"
	OpIn                 = "IN"
	OpIsNull             = "NULL"
	OpIsNotNull          = "NOT NULL"
)
