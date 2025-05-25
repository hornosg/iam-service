package criteria

// Criteria representa un conjunto de criterios de filtrado, ordenación y paginación
type Criteria struct {
	Filters    Filters
	Order      Order
	Pagination Pagination
}

// NewCriteria crea una nueva instancia de Criteria
func NewCriteria(filters Filters, order Order, pagination Pagination) Criteria {
	return Criteria{
		Filters:    filters,
		Order:      order,
		Pagination: pagination,
	}
}

// HasFilters retorna true si hay filtros definidos
func (c *Criteria) HasFilters() bool {
	return len(c.Filters.Items) > 0
}

// HasOrder retorna true si hay ordenamiento definido
func (c *Criteria) HasOrder() bool {
	return c.Order.Field != ""
}

// HasPagination retorna true si hay paginación definida
func (c *Criteria) HasPagination() bool {
	return c.Pagination.Limit > 0
}

// Filter representa un filtro individual
type Filter struct {
	Field    string
	Operator string
	Value    interface{}
}

// NewFilter crea un nuevo filtro
func NewFilter(field string, operator string, value interface{}) Filter {
	return Filter{
		Field:    field,
		Operator: operator,
		Value:    value,
	}
}

// Filters representa una colección de filtros
type Filters struct {
	Items []Filter
}

// NewFilters crea una nueva colección de filtros
func NewFilters(items ...Filter) Filters {
	return Filters{
		Items: items,
	}
}

// Add agrega un filtro a la colección
func (f *Filters) Add(filter Filter) {
	f.Items = append(f.Items, filter)
}

// AddFilter agrega un filtro usando los parámetros individuales
func (f *Filters) AddFilter(field, operator string, value interface{}) {
	f.Add(NewFilter(field, operator, value))
}

// HasField verifica si existe un filtro para el campo especificado
func (f *Filters) HasField(field string) bool {
	for _, filter := range f.Items {
		if filter.Field == field {
			return true
		}
	}
	return false
}

// GetByField obtiene el primer filtro que coincida con el campo
func (f *Filters) GetByField(field string) (*Filter, bool) {
	for _, filter := range f.Items {
		if filter.Field == field {
			return &filter, true
		}
	}
	return nil, false
}

// Order representa el criterio de ordenación
type Order struct {
	Field     string
	Direction string
}

// NewOrder crea un nuevo criterio de ordenación
func NewOrder(field string, direction string) Order {
	return Order{
		Field:     field,
		Direction: direction,
	}
}

// NewOrderAsc crea un ordenamiento ascendente
func NewOrderAsc(field string) Order {
	return NewOrder(field, "ASC")
}

// NewOrderDesc crea un ordenamiento descendente
func NewOrderDesc(field string) Order {
	return NewOrder(field, "DESC")
}

// IsValid verifica si el orden es válido
func (o *Order) IsValid() bool {
	return o.Field != "" && (o.Direction == "ASC" || o.Direction == "DESC")
}

// Pagination representa los criterios de paginación
type Pagination struct {
	Limit  int
	Offset int
}

// NewPagination crea un nuevo criterio de paginación
func NewPagination(limit int, offset int) Pagination {
	return Pagination{
		Limit:  limit,
		Offset: offset,
	}
}

// NewPaginationFromPage crea paginación basada en página y tamaño
func NewPaginationFromPage(page, pageSize int) Pagination {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize
	return NewPagination(pageSize, offset)
}

// GetPage calcula el número de página actual
func (p *Pagination) GetPage() int {
	if p.Limit <= 0 {
		return 1
	}
	return (p.Offset / p.Limit) + 1
}

// GetPageSize retorna el tamaño de página
func (p *Pagination) GetPageSize() int {
	return p.Limit
}

// IsValid verifica si la paginación es válida
func (p *Pagination) IsValid() bool {
	return p.Limit > 0 && p.Offset >= 0
}
