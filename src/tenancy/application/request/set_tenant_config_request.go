package request

// SetTenantConfigRequest representa el DTO de entrada para establecer una configuración
type SetTenantConfigRequest struct {
	Key   string `json:"key" binding:"required"`
	Value string `json:"value" binding:"required"`
}

// Validate valida que los campos no estén vacíos
func (r *SetTenantConfigRequest) Validate() error {
	if r.Key == "" {
		return &ValidationError{Field: "key", Message: "key cannot be empty"}
	}
	if r.Value == "" {
		return &ValidationError{Field: "value", Message: "value cannot be empty"}
	}
	return nil
}

// ValidationError representa un error de validación
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}
