package request

type UpdateRoleRequest struct {
	Name        *string   `json:"name,omitempty" binding:"omitempty,min=2,max=100"`
	Description *string   `json:"description,omitempty" binding:"omitempty,min=5,max=500"`
	IsActive    *bool     `json:"is_active,omitempty"`
	Permissions *[]string `json:"permissions,omitempty"`
}
