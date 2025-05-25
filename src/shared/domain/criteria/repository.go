package criteria

import (
	"context"
)

// CriteriaRepository define la interfaz para repositorios que soporten búsqueda por criterios
type CriteriaRepository[T any] interface {
	// SearchByCriteria busca entidades según los criterios especificados
	SearchByCriteria(ctx context.Context, criteria Criteria) ([]*T, error)

	// CountByCriteria cuenta entidades según los criterios especificados (sin paginación)
	CountByCriteria(ctx context.Context, criteria Criteria) (int, error)
}

// ListRepository define operaciones básicas de listado
type ListRepository[T any] interface {
	// List obtiene todas las entidades con paginación
	List(ctx context.Context, limit, offset int) ([]*T, error)

	// Count obtiene el total de entidades
	Count(ctx context.Context) (int, error)
}

// AdvancedCriteriaRepository combina ambas interfaces
type AdvancedCriteriaRepository[T any] interface {
	CriteriaRepository[T]
	ListRepository[T]
}

// ListResponse representa una respuesta paginada genérica
type ListResponse[T any] struct {
	Items      []*T `json:"items"`
	TotalCount int  `json:"total_count"`
	Page       int  `json:"page"`
	PageSize   int  `json:"page_size"`
	TotalPages int  `json:"total_pages"`
}

// NewListResponse crea una nueva respuesta de lista
func NewListResponse[T any](items []*T, totalCount int, criteria Criteria) *ListResponse[T] {
	page := criteria.Pagination.GetPage()
	pageSize := criteria.Pagination.GetPageSize()
	totalPages := (totalCount + pageSize - 1) / pageSize // Redondeo hacia arriba

	return &ListResponse[T]{
		Items:      items,
		TotalCount: totalCount,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

// HasNextPage verifica si hay una página siguiente
func (lr *ListResponse[T]) HasNextPage() bool {
	return lr.Page < lr.TotalPages
}

// HasPrevPage verifica si hay una página anterior
func (lr *ListResponse[T]) HasPrevPage() bool {
	return lr.Page > 1
}

// IsEmpty verifica si la respuesta está vacía
func (lr *ListResponse[T]) IsEmpty() bool {
	return len(lr.Items) == 0
}
