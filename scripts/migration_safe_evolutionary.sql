-- Migración Evolutiva Segura para IAM
-- Actualiza el esquema existente hacia la nueva arquitectura modular
-- ⚠️  PROTEGE contraseñas existentes - NO las modifica
-- ✅ Mantiene TODOS los datos existentes

-- =================================================
-- 1. EVOLUCIÓN DE LA TABLA ROLES
-- =================================================
-- Nueva columna necesaria para arquitectura modular:
-- - permissions: array de permisos granulares ["users:read", "users:write", etc.]
-- 
-- NOTA: NO necesitamos tenant_id ni is_system
-- Todos los roles son genéricos del sistema y funcionales para cualquier negocio

ALTER TABLE roles ADD COLUMN IF NOT EXISTS permissions TEXT[] DEFAULT ARRAY[]::text[];

-- Asignar permisos inteligentes según el nombre del rol existente
UPDATE roles SET permissions = ARRAY['*'] 
WHERE (name ILIKE '%admin%' OR name ILIKE '%super%') 
AND (permissions IS NULL OR array_length(permissions, 1) = 0);

UPDATE roles SET permissions = ARRAY['users:read', 'users:write', 'roles:read', 'tenants:read', 'plans:read'] 
WHERE (name ILIKE '%manager%' OR name ILIKE '%gerente%') 
AND (permissions IS NULL OR array_length(permissions, 1) = 0);

UPDATE roles SET permissions = ARRAY['profile:read', 'profile:write', 'dashboard:read'] 
WHERE (name ILIKE '%user%' OR name ILIKE '%usuario%' OR name ILIKE '%basic%' OR name ILIKE '%employee%') 
AND (permissions IS NULL OR array_length(permissions, 1) = 0);

UPDATE roles SET permissions = ARRAY['dashboard:read'] 
WHERE (name ILIKE '%view%' OR name ILIKE '%read%' OR name ILIKE '%guest%') 
AND (permissions IS NULL OR array_length(permissions, 1) = 0);

-- Si no coincide con ningún patrón, asignar permisos básicos
UPDATE roles SET permissions = ARRAY['profile:read', 'profile:write'] 
WHERE (permissions IS NULL OR array_length(permissions, 1) = 0);

-- =================================================
-- 2. EVOLUCIÓN DE LA TABLA TENANTS
-- =================================================
-- Nuevas columnas para arquitectura modular:
-- - slug: identificador único para subdominios (ej: empresa-123.miapp.com)
-- - type: tipo de tenant (PERSONAL, STARTUP, BUSINESS, ENTERPRISE)
-- - status: estado del tenant (ACTIVE, INACTIVE, SUSPENDED, DELETED)
-- - owner_id: usuario propietario del tenant
-- - user_count: contador de usuarios para límites
-- - settings: configuraciones personalizadas en JSON

ALTER TABLE tenants ADD COLUMN IF NOT EXISTS slug VARCHAR(100);
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS type VARCHAR(50) DEFAULT 'BUSINESS';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS status VARCHAR(50) DEFAULT 'ACTIVE';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS owner_id UUID;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS domain VARCHAR(255);
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS user_count INTEGER DEFAULT 0;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS max_users INTEGER;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS settings JSONB DEFAULT '{}';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS expires_at TIMESTAMP WITH TIME ZONE;

-- Crear slug a partir del nombre si no existe
UPDATE tenants SET slug = lower(regexp_replace(regexp_replace(name, '[^a-zA-Z0-9\s]', '', 'g'), '\s+', '-', 'g'))
WHERE slug IS NULL AND name IS NOT NULL;

-- Asegurar que todos los slugs sean únicos y válidos
DO $$
DECLARE
    r RECORD;
    counter INTEGER;
    new_slug TEXT;
BEGIN
    FOR r IN SELECT id, slug, name FROM tenants WHERE slug IS NOT NULL LOOP
        counter := 1;
        new_slug := r.slug;
        
        -- Asegurar que el slug no esté vacío
        IF new_slug = '' OR new_slug IS NULL THEN
            new_slug := 'tenant-' || substr(r.id::text, 1, 8);
        END IF;
        
        -- Si el slug ya existe, añadir número
        WHILE EXISTS (SELECT 1 FROM tenants WHERE slug = new_slug AND id != r.id) LOOP
            new_slug := COALESCE(r.slug, 'tenant') || '-' || counter;
            counter := counter + 1;
        END LOOP;
        
        -- Actualizar si cambió
        IF new_slug != COALESCE(r.slug, '') THEN
            UPDATE tenants SET slug = new_slug WHERE id = r.id;
        END IF;
    END LOOP;
END $$;

-- Agregar constraints después de limpiar datos
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'tenants_type_check') THEN
        ALTER TABLE tenants ADD CONSTRAINT tenants_type_check CHECK (type IN ('PERSONAL', 'STARTUP', 'BUSINESS', 'ENTERPRISE'));
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'tenants_status_check') THEN
        ALTER TABLE tenants ADD CONSTRAINT tenants_status_check CHECK (status IN ('ACTIVE', 'INACTIVE', 'SUSPENDED', 'DELETED'));
    END IF;
END $$;

-- Crear índices para tenants si no existen
CREATE INDEX IF NOT EXISTS idx_tenants_owner_id ON tenants(owner_id);
CREATE INDEX IF NOT EXISTS idx_tenants_status ON tenants(status);
CREATE INDEX IF NOT EXISTS idx_tenants_type ON tenants(type);
CREATE INDEX IF NOT EXISTS idx_tenants_plan_id ON tenants(plan_id);
CREATE INDEX IF NOT EXISTS idx_tenants_expires_at ON tenants(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_tenants_slug ON tenants(slug);
CREATE INDEX IF NOT EXISTS idx_tenants_domain ON tenants(domain) WHERE domain IS NOT NULL;

-- Migrar el email_user_key a owner_id (buscar usuario por email)
-- Solo si owner_id es null y tenemos email_user_key
UPDATE tenants 
SET owner_id = (
    SELECT id FROM users 
    WHERE email = tenants.email_user_key 
    LIMIT 1
)
WHERE owner_id IS NULL AND email_user_key IS NOT NULL;

-- Contar usuarios por tenant
UPDATE tenants 
SET user_count = (
    SELECT COUNT(*) FROM users 
    WHERE users.tenant_id = tenants.id
)
WHERE user_count = 0 OR user_count IS NULL;

-- =================================================
-- 3. EVOLUCIÓN DE LA TABLA USERS
-- =================================================
-- ⚠️  IMPORTANTE: NO modificamos password_hash existente
-- Nuevas columnas para arquitectura modular:
-- - first_name, last_name: nombres separados
-- - is_active, is_verified: estados booleanos más claros

ALTER TABLE users ADD COLUMN IF NOT EXISTS first_name VARCHAR(100);
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_name VARCHAR(100);
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_active BOOLEAN DEFAULT TRUE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_verified BOOLEAN DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login TIMESTAMP WITH TIME ZONE;

-- Migrar datos del status actual al nuevo sistema SOLO si las columnas están vacías
-- ⚠️  NO tocamos password_hash NUNCA
UPDATE users SET is_active = (
    CASE 
        WHEN status = 'ACTIVE' THEN TRUE
        WHEN status = 'INACTIVE' THEN FALSE
        ELSE TRUE  -- default seguro
    END
) WHERE is_active IS NULL;

UPDATE users SET is_verified = (
    CASE 
        WHEN status = 'ACTIVE' THEN TRUE
        ELSE FALSE
    END
) WHERE is_verified IS NULL;

-- Crear índices para users si no existen
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_users_role_id ON users(role_id);
CREATE INDEX IF NOT EXISTS idx_users_is_active ON users(is_active);

-- =================================================
-- 4. FUNCIONES Y TRIGGERS
-- =================================================

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Triggers para todas las tablas (solo si no existen)
DO $$ 
BEGIN 
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'update_plans_updated_at') THEN
        CREATE TRIGGER update_plans_updated_at 
            BEFORE UPDATE ON plans 
            FOR EACH ROW 
            EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

DO $$ 
BEGIN 
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'update_roles_updated_at') THEN
        CREATE TRIGGER update_roles_updated_at 
            BEFORE UPDATE ON roles 
            FOR EACH ROW 
            EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

DO $$ 
BEGIN 
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'update_tenants_updated_at') THEN
        CREATE TRIGGER update_tenants_updated_at 
            BEFORE UPDATE ON tenants 
            FOR EACH ROW 
            EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

DO $$ 
BEGIN 
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'update_users_updated_at') THEN
        CREATE TRIGGER update_users_updated_at 
            BEFORE UPDATE ON users 
            FOR EACH ROW 
            EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

-- =================================================
-- 5. ROLES GENÉRICOS DEL SISTEMA
-- =================================================
-- Roles estratégicos que funcionan para cualquier tipo de negocio
-- (CRM, ERP, E-commerce, etc.) - simples y efectivos

-- Super Administrador (control total)
INSERT INTO roles (name, description, permissions, saas)
SELECT 'super_admin', 'Súper Administrador - Control total del sistema', ARRAY['*'], 'CRM'
WHERE NOT EXISTS (SELECT 1 FROM roles WHERE name = 'super_admin');

-- Administrador de Empresa/Tenant
INSERT INTO roles (name, description, permissions, saas)
SELECT 'admin', 'Administrador de Empresa - Gestión completa del tenant', 
       ARRAY['users:read', 'users:write', 'users:delete', 'roles:read', 'tenants:read', 'tenants:write', 'plans:read', 'dashboard:read', 'reports:read'], 
       'CRM'
WHERE NOT EXISTS (SELECT 1 FROM roles WHERE name = 'admin');

-- Gerente/Manager (permisos avanzados)
INSERT INTO roles (name, description, permissions, saas)
SELECT 'manager', 'Gerente - Supervisión y gestión de equipos', 
       ARRAY['users:read', 'users:write', 'roles:read', 'tenants:read', 'dashboard:read', 'reports:read', 'analytics:read'], 
       'CRM'
WHERE NOT EXISTS (SELECT 1 FROM roles WHERE name = 'manager');

-- Empleado (permisos operativos)
INSERT INTO roles (name, description, permissions, saas)
SELECT 'employee', 'Empleado - Operaciones diarias del negocio', 
       ARRAY['profile:read', 'profile:write', 'dashboard:read', 'customers:read', 'customers:write', 'orders:read', 'orders:write'], 
       'CRM'
WHERE NOT EXISTS (SELECT 1 FROM roles WHERE name = 'employee');

-- Visualizador (solo lectura)
INSERT INTO roles (name, description, permissions, saas)
SELECT 'viewer', 'Visualizador - Solo lectura y consultas', 
       ARRAY['dashboard:read', 'reports:read', 'profile:read'], 
       'CRM'
WHERE NOT EXISTS (SELECT 1 FROM roles WHERE name = 'viewer');

-- =================================================
-- 6. VERIFICACIONES POST-MIGRACIÓN
-- =================================================

DO $$ 
DECLARE
    user_count INTEGER;
    tenant_count INTEGER;
    role_count INTEGER;
BEGIN 
    SELECT COUNT(*) INTO user_count FROM users;
    SELECT COUNT(*) INTO tenant_count FROM tenants;
    SELECT COUNT(*) INTO role_count FROM roles;
    
    RAISE NOTICE '🎉 Migración evolutiva completada exitosamente!';
    RAISE NOTICE '📊 Estado final:';
    RAISE NOTICE '   - Users: % registros (contraseñas protegidas)', user_count;
    RAISE NOTICE '   - Tenants: % registros (con slugs únicos)', tenant_count;
    RAISE NOTICE '   - Roles: % registros (genéricos simplificados)', role_count;
    RAISE NOTICE '✅ Esquema actualizado para nueva arquitectura modular';
    RAISE NOTICE '🔐 Contraseñas existentes NO modificadas';
    RAISE NOTICE '🎯 Roles genéricos y simples para cualquier negocio';
    RAISE NOTICE '🚀 Arquitectura simplificada sin complejidades innecesarias';
END $$; 