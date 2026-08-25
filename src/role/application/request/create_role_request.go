package request

import (
	"iam/src/role/domain/value_object"
)

// CreateRoleRequest模型 la creación de un rol.
//
// ACC-E02 T10: `roles` es un catálogo global sin `tenant_id`, por lo que el
// request ya no acepta `tenant_id` del body. El tipo de rol (incluido
// `SYSTEM_ADMIN`) se sigue enviando, pero la escritura está gated por
// `system:admin` a nivel de ruta (main.go), que es la defensa real.
type CreateRoleRequest struct {
	Name        string   `json:"name" binding:"required,min=2,max=100"`
	Description string   `json:"description" binding:"required,min=5,max=500"`
	Type        string   `json:"type" binding:"required,oneof=SYSTEM_ADMIN TENANT_ADMIN USER READ_ONLY CUSTOM"`
	Permissions []string `json:"permissions,omitempty"`
}

func (r *CreateRoleRequest) GetRoleType() (value_object.RoleType, error) {
	return value_object.NewRoleTypeFromString(r.Type)
}
