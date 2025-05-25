package criteria

import (
	"fmt"
	"strings"

	domainCriteria "iam/src/shared/domain/criteria"
)

// SQLCriteriaConverter convierte un objeto Criteria en una consulta SQL
type SQLCriteriaConverter struct {
	paramIndex int
}

// NewSQLCriteriaConverter crea una nueva instancia del conversor
func NewSQLCriteriaConverter() *SQLCriteriaConverter {
	return &SQLCriteriaConverter{
		paramIndex: 0,
	}
}

// ToSQL convierte un criteria a una consulta SQL con sus parámetros
func (s *SQLCriteriaConverter) ToSQL(criteria domainCriteria.Criteria) (string, []interface{}) {
	s.paramIndex = 0 // Reset parameter index
	var conditions []string
	var params []interface{}

	// Procesar los filtros
	for _, filter := range criteria.Filters.Items {
		condition, values := s.processFilter(filter)
		if condition != "" {
			conditions = append(conditions, condition)
			params = append(params, values...)
		}
	}

	// Construir la cláusula WHERE
	var whereClause string
	if len(conditions) > 0 {
		whereClause = fmt.Sprintf("WHERE %s", strings.Join(conditions, " AND "))
	}

	// Construir la cláusula ORDER BY
	var orderByClause string
	if criteria.Order.Field != "" {
		orderByClause = fmt.Sprintf("ORDER BY %s %s", criteria.Order.Field, criteria.Order.Direction)
	}

	// Construir la cláusula LIMIT y OFFSET
	var limitOffsetClause string
	if criteria.Pagination.Limit > 0 {
		limitOffsetClause = fmt.Sprintf("LIMIT %d OFFSET %d", criteria.Pagination.Limit, criteria.Pagination.Offset)
	}

	// Combinar las cláusulas
	clauses := []string{whereClause, orderByClause, limitOffsetClause}
	var filteredClauses []string
	for _, clause := range clauses {
		if clause != "" {
			filteredClauses = append(filteredClauses, clause)
		}
	}

	return strings.Join(filteredClauses, " "), params
}

// ToSelectSQL construye una query SELECT completa con tabla base
func (s *SQLCriteriaConverter) ToSelectSQL(baseQuery string, criteria domainCriteria.Criteria) (string, []interface{}) {
	clausesPart, params := s.ToSQL(criteria)

	if clausesPart == "" {
		return baseQuery, params
	}

	return fmt.Sprintf("%s %s", baseQuery, clausesPart), params
}

// ToCountSQL construye una query COUNT con los filtros aplicados
func (s *SQLCriteriaConverter) ToCountSQL(baseCountQuery string, criteria domainCriteria.Criteria) (string, []interface{}) {
	s.paramIndex = 0 // Reset parameter index
	var conditions []string
	var params []interface{}

	// Solo procesar filtros para COUNT (sin ORDER BY ni LIMIT)
	for _, filter := range criteria.Filters.Items {
		condition, values := s.processFilter(filter)
		if condition != "" {
			conditions = append(conditions, condition)
			params = append(params, values...)
		}
	}

	if len(conditions) > 0 {
		whereClause := fmt.Sprintf("WHERE %s", strings.Join(conditions, " AND "))
		return fmt.Sprintf("%s %s", baseCountQuery, whereClause), params
	}

	return baseCountQuery, params
}

// processFilter convierte un filtro en una condición SQL
func (s *SQLCriteriaConverter) processFilter(filter domainCriteria.Filter) (string, []interface{}) {
	var condition string
	var params []interface{}

	switch filter.Operator {
	case domainCriteria.OpEqual, domainCriteria.OpNotEqual,
		domainCriteria.OpGreaterThan, domainCriteria.OpGreaterThanOrEqual,
		domainCriteria.OpLessThan, domainCriteria.OpLessThanOrEqual:
		s.paramIndex++
		condition = fmt.Sprintf("%s %s $%d", filter.Field, filter.Operator, s.paramIndex)
		params = append(params, filter.Value)

	case domainCriteria.OpLike:
		s.paramIndex++
		condition = fmt.Sprintf("%s LIKE $%d", filter.Field, s.paramIndex)
		// Asegurar que el valor sea compatible con LIKE
		likeValue := s.prepareLikeValue(filter.Value)
		params = append(params, likeValue)

	case domainCriteria.OpIn:
		// Manejar arrays para cláusulas IN
		if values, ok := s.convertToSlice(filter.Value); ok && len(values) > 0 {
			placeholders := make([]string, len(values))
			for i, val := range values {
				s.paramIndex++
				placeholders[i] = fmt.Sprintf("$%d", s.paramIndex)
				params = append(params, val)
			}
			condition = fmt.Sprintf("%s IN (%s)", filter.Field, strings.Join(placeholders, ", "))
		}

	case domainCriteria.OpIsNull:
		condition = fmt.Sprintf("%s IS NULL", filter.Field)

	case domainCriteria.OpIsNotNull:
		condition = fmt.Sprintf("%s IS NOT NULL", filter.Field)

	default:
		// Operador por defecto: igualdad
		s.paramIndex++
		condition = fmt.Sprintf("%s = $%d", filter.Field, s.paramIndex)
		params = append(params, filter.Value)
	}

	return condition, params
}

// prepareLikeValue prepara un valor para ser usado con LIKE
func (s *SQLCriteriaConverter) prepareLikeValue(value interface{}) interface{} {
	if str, ok := value.(string); ok {
		// Si no contiene wildcards, agregar % al inicio y final
		if !strings.Contains(str, "%") && !strings.Contains(str, "_") {
			return "%" + str + "%"
		}
		return str
	}
	return value
}

// convertToSlice convierte diferentes tipos a []interface{}
func (s *SQLCriteriaConverter) convertToSlice(value interface{}) ([]interface{}, bool) {
	switch v := value.(type) {
	case []interface{}:
		return v, true
	case []string:
		result := make([]interface{}, len(v))
		for i, str := range v {
			result[i] = str
		}
		return result, true
	case []int:
		result := make([]interface{}, len(v))
		for i, num := range v {
			result[i] = num
		}
		return result, true
	case string:
		// Si es un string separado por comas, dividirlo
		if strings.Contains(v, ",") {
			parts := strings.Split(v, ",")
			result := make([]interface{}, len(parts))
			for i, part := range parts {
				result[i] = strings.TrimSpace(part)
			}
			return result, true
		}
		return []interface{}{v}, true
	default:
		return nil, false
	}
}

// BuildWhereConditions construye solo las condiciones WHERE (sin la palabra WHERE)
func (s *SQLCriteriaConverter) BuildWhereConditions(criteria domainCriteria.Criteria) (string, []interface{}) {
	s.paramIndex = 0
	var conditions []string
	var params []interface{}

	for _, filter := range criteria.Filters.Items {
		condition, values := s.processFilter(filter)
		if condition != "" {
			conditions = append(conditions, condition)
			params = append(params, values...)
		}
	}

	if len(conditions) > 0 {
		return strings.Join(conditions, " AND "), params
	}

	return "", params
}

// GetNextParamIndex retorna el próximo índice de parámetro disponible
func (s *SQLCriteriaConverter) GetNextParamIndex() int {
	s.paramIndex++
	return s.paramIndex
}
