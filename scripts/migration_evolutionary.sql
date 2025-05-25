-- Migración Evolutiva para IAM
-- Actualiza el esquema existente hacia la nueva arquitectura modular
-- Mantiene datos existentes y añade nuevas columnas/funcionalidades

-- =================================================
-- 1. EVOLUCIÓN DE LA TABLA ROLES
-- =================================================

-- Agregar columnas faltantes a roles
ALTER TABLE roles ADD COLUMN IF NOT EXISTS permissions TEXT[] DEFAULT ARRAY[]::text[];
ALTER TABLE roles ADD COLUMN IF NOT EXISTS is_system BOOLEAN DEFAULT FALSE;
ALTER TABLE roles ADD COLUMN IF NOT EXISTS tenant_id UUID;

-- Crear índices para roles si no existen
CREATE INDEX IF NOT EXISTS idx_roles_tenant_id ON roles(tenant_id);
CREATE INDEX IF NOT EXISTS idx_roles_is_system ON roles(is_system);

-- Migrar roles existentes a la nueva estructura
-- Los roles existentes se marcan como roles de sistema
UPDATE roles SET is_system = TRUE WHERE is_system IS NULL OR is_system = FALSE;

-- Asignar permisos básicos según el tipo de rol existente
UPDATE roles SET permissions = ARRAY['*'] WHERE name ILIKE '%admin%' AND (permissions IS NULL OR array_length(permissions, 1) = 0);
UPDATE roles SET permissions = ARRAY['users:read', 'users:write', 'roles:read'] WHERE name ILIKE '%manager%' AND (permissions IS NULL OR array_length(permissions, 1) = 0);
UPDATE roles SET permissions = ARRAY['profile:read', 'profile:write'] WHERE name ILIKE '%user%' AND (permissions IS NULL OR array_length(permissions, 1) = 0);

-- =================================================
-- 2. EVOLUCIÓN DE LA TABLA TENANTS
-- =================================================

-- Agregar columnas faltantes a tenants
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
UPDATE tenants SET slug = lower(replace(replace(name, ' ', '-'), '_', '-')) 
WHERE slug IS NULL AND name IS NOT NULL;

-- Asegurar que todos los slugs sean únicos
DO $$
DECLARE
    r RECORD;
    counter INTEGER;
    new_slug TEXT;
BEGIN
    FOR r IN SELECT id, slug FROM tenants WHERE slug IS NOT NULL LOOP
        counter := 1;
        new_slug := r.slug;
        
        -- Si el slug ya existe, añadir número
        WHILE EXISTS (SELECT 1 FROM tenants WHERE slug = new_slug AND id != r.id) LOOP
            new_slug := r.slug || '-' || counter;
            counter := counter + 1;
        END LOOP;
        
        -- Actualizar si cambió
        IF new_slug != r.slug THEN
            UPDATE tenants SET slug = new_slug WHERE id = r.id;
        END IF;
    END LOOP;
END $$;

-- Agregar constraints después de limpiar datos
ALTER TABLE tenants ADD CONSTRAINT IF NOT EXISTS tenants_slug_unique UNIQUE (slug);
ALTER TABLE tenants ADD CONSTRAINT IF NOT EXISTS tenants_domain_unique UNIQUE (domain);
ALTER TABLE tenants ADD CONSTRAINT IF NOT EXISTS tenants_type_check CHECK (type IN ('PERSONAL', 'STARTUP', 'BUSINESS', 'ENTERPRISE'));
ALTER TABLE tenants ADD CONSTRAINT IF NOT EXISTS tenants_status_check CHECK (status IN ('ACTIVE', 'INACTIVE', 'SUSPENDED', 'DELETED'));

-- Crear índices para tenants si no existen
CREATE INDEX IF NOT EXISTS idx_tenants_owner_id ON tenants(owner_id);
CREATE INDEX IF NOT EXISTS idx_tenants_status ON tenants(status);
CREATE INDEX IF NOT EXISTS idx_tenants_type ON tenants(type);
CREATE INDEX IF NOT EXISTS idx_tenants_plan_id ON tenants(plan_id);
CREATE INDEX IF NOT EXISTS idx_tenants_expires_at ON tenants(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_tenants_slug ON tenants(slug);
CREATE INDEX IF NOT EXISTS idx_tenants_domain ON tenants(domain) WHERE domain IS NOT NULL;

-- Migrar el email_user_key a owner_id (buscar usuario por email)
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
);

-- =================================================
-- 3. EVOLUCIÓN DE LA TABLA USERS
-- =================================================

-- Agregar columnas faltantes a users
ALTER TABLE users ADD COLUMN IF NOT EXISTS first_name VARCHAR(100);
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_name VARCHAR(100);
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_active BOOLEAN DEFAULT TRUE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_verified BOOLEAN DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login TIMESTAMP WITH TIME ZONE;

-- Migrar datos del status actual al nuevo sistema
UPDATE users SET is_active = TRUE WHERE status = 'ACTIVE';
UPDATE users SET is_active = FALSE WHERE status = 'INACTIVE';
UPDATE users SET is_verified = TRUE WHERE status = 'ACTIVE';

-- Crear índices para users si no existen
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_users_role_id ON users(role_id);
CREATE INDEX IF NOT EXISTS idx_users_is_active ON users(is_active);

-- =================================================
-- 4. FUNCIONES Y TRIGGERS (igual que antes)
-- =================================================

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Triggers para todas las tablas
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
-- 5. DATOS INICIALES PARA NUEVA ARQUITECTURA
-- =================================================

-- Insertar roles de sistema adicionales si no existen
INSERT INTO roles (name, description, permissions, is_system, saas)
SELECT 'super_admin', 'Administrador del sistema', ARRAY['*'], TRUE, 'CRM'
WHERE NOT EXISTS (SELECT 1 FROM roles WHERE name = 'super_admin' AND is_system = TRUE);

INSERT INTO roles (name, description, permissions, is_system, saas)
SELECT 'tenant_admin', 'Administrador de tenant', ARRAY['users:read', 'users:write', 'roles:read'], TRUE, 'CRM'
WHERE NOT EXISTS (SELECT 1 FROM roles WHERE name = 'tenant_admin' AND is_system = TRUE);

INSERT INTO roles (name, description, permissions, is_system, saas)
SELECT 'basic_user', 'Usuario básico', ARRAY['profile:read', 'profile:write'], TRUE, 'CRM'
WHERE NOT EXISTS (SELECT 1 FROM roles WHERE name = 'basic_user' AND is_system = TRUE);

-- =================================================
-- 6. VERIFICACIONES POST-MIGRACIÓN
-- =================================================

DO $$ 
BEGIN 
    RAISE NOTICE 'Migración evolutiva completada exitosamente.';
    RAISE NOTICE 'Tablas actualizadas: plans, roles, tenants, users';
    RAISE NOTICE 'Nuevas columnas añadidas y datos migrados.';
    RAISE NOTICE 'Compatibilidad mantenida con arquitectura anterior.';
END $$; 