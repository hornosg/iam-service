-- Migration: Create roles table
-- Description: Tabla para almacenar roles del sistema

CREATE TABLE IF NOT EXISTS roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL,
    type VARCHAR(20) NOT NULL,
    tenant_id UUID, -- NULL para roles de sistema
    permissions TEXT[], -- Array de permisos
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT roles_type_check CHECK (type IN ('SYSTEM_ADMIN', 'TENANT_ADMIN', 'USER', 'READ_ONLY', 'CUSTOM')),
    CONSTRAINT roles_name_tenant_unique UNIQUE (name, tenant_id) -- Nombre único por tenant
);

-- Indexes para optimizar consultas
CREATE INDEX IF NOT EXISTS idx_roles_type ON roles(type);
CREATE INDEX IF NOT EXISTS idx_roles_tenant_id ON roles(tenant_id);
CREATE INDEX IF NOT EXISTS idx_roles_is_active ON roles(is_active);
CREATE INDEX IF NOT EXISTS idx_roles_name ON roles(name);
CREATE INDEX IF NOT EXISTS idx_roles_created_at ON roles(created_at);

-- Trigger para actualizar updated_at automáticamente
CREATE TRIGGER roles_update_updated_at 
    BEFORE UPDATE ON roles 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

-- Insertar roles de sistema por defecto
INSERT INTO roles (name, description, type, tenant_id, permissions) VALUES
('System Administrator', 'Full system access', 'SYSTEM_ADMIN', NULL, ARRAY['*']),
('Tenant Administrator', 'Full tenant access', 'TENANT_ADMIN', NULL, ARRAY['tenant:*', 'user:*', 'role:*']),
('Regular User', 'Standard user access', 'USER', NULL, ARRAY['user:read', 'user:update_own']),
('Read Only User', 'Read-only access', 'READ_ONLY', NULL, ARRAY['user:read'])
ON CONFLICT (name, tenant_id) DO NOTHING; 