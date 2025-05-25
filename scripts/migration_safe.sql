-- Migración Segura e Idempotente para IAM
-- Este script verifica que las tablas no existan antes de crearlas
-- y solo aplica las modificaciones necesarias

-- =================================================
-- 1. CREAR TIPOS ENUM (solo si no existen)
-- =================================================

DO $$ 
BEGIN 
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'saas_type') THEN
        CREATE TYPE saas_type AS ENUM ('CRM', 'ERP', 'ECOMMERCE');
    END IF;
END $$;

-- =================================================
-- 2. TABLA PLANS (solo si no existe)
-- =================================================

CREATE TABLE IF NOT EXISTS plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    saas saas_type NOT NULL,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    features TEXT[] DEFAULT ARRAY[]::text[],
    monthly_price DECIMAL(10,2) NOT NULL,
    yearly_price DECIMAL(10,2) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- =================================================
-- 3. TABLA ROLES (solo si no existe)
-- =================================================

CREATE TABLE IF NOT EXISTS roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    permissions TEXT[] DEFAULT ARRAY[]::text[],
    is_system BOOLEAN DEFAULT FALSE,
    tenant_id UUID,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Crear índices para roles si no existen
CREATE INDEX IF NOT EXISTS idx_roles_tenant_id ON roles(tenant_id);
CREATE INDEX IF NOT EXISTS idx_roles_is_system ON roles(is_system);

-- =================================================
-- 4. TABLA TENANTS (solo si no existe)
-- =================================================

CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    type VARCHAR(50) NOT NULL CHECK (type IN ('PERSONAL', 'STARTUP', 'BUSINESS', 'ENTERPRISE')),
    status VARCHAR(50) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE', 'SUSPENDED', 'DELETED')),
    owner_id UUID NOT NULL,
    domain VARCHAR(255) UNIQUE,
    plan_id UUID,
    user_count INTEGER NOT NULL DEFAULT 0,
    max_users INTEGER,
    settings JSONB DEFAULT '{}',
    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Crear índices para tenants si no existen
CREATE INDEX IF NOT EXISTS idx_tenants_owner_id ON tenants(owner_id);
CREATE INDEX IF NOT EXISTS idx_tenants_status ON tenants(status);
CREATE INDEX IF NOT EXISTS idx_tenants_type ON tenants(type);
CREATE INDEX IF NOT EXISTS idx_tenants_plan_id ON tenants(plan_id);
CREATE INDEX IF NOT EXISTS idx_tenants_expires_at ON tenants(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_tenants_slug ON tenants(slug);
CREATE INDEX IF NOT EXISTS idx_tenants_domain ON tenants(domain) WHERE domain IS NOT NULL;

-- =================================================
-- 5. TABLA USERS (solo si no existe)
-- =================================================

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    is_active BOOLEAN DEFAULT TRUE,
    is_verified BOOLEAN DEFAULT FALSE,
    tenant_id UUID,
    role_id UUID,
    last_login TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Crear índices para users si no existen
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_users_role_id ON users(role_id);
CREATE INDEX IF NOT EXISTS idx_users_is_active ON users(is_active);

-- =================================================
-- 6. FUNCIÓN PARA UPDATE AUTOMÁTICO (solo si no existe)
-- =================================================

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- =================================================
-- 7. TRIGGERS (solo si no existen)
-- =================================================

-- Trigger para plans
DO $$ 
BEGIN 
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'update_plans_updated_at') THEN
        CREATE TRIGGER update_plans_updated_at 
            BEFORE UPDATE ON plans 
            FOR EACH ROW 
            EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

-- Trigger para roles
DO $$ 
BEGIN 
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'update_roles_updated_at') THEN
        CREATE TRIGGER update_roles_updated_at 
            BEFORE UPDATE ON roles 
            FOR EACH ROW 
            EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

-- Trigger para tenants
DO $$ 
BEGIN 
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'update_tenants_updated_at') THEN
        CREATE TRIGGER update_tenants_updated_at 
            BEFORE UPDATE ON tenants 
            FOR EACH ROW 
            EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

-- Trigger para users
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
-- 8. VERIFICAR QUE COLUMNAS ADICIONALES EXISTAN
-- =================================================

-- Agregar columna features si no existe en plans
DO $$ 
BEGIN 
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'plans' AND column_name = 'features' AND table_schema = 'public'
    ) THEN
        ALTER TABLE plans ADD COLUMN features TEXT[] DEFAULT ARRAY[]::text[];
    END IF;
END $$;

-- =================================================
-- 9. DATOS INICIALES SEGUROS (solo si no existen)
-- =================================================

-- Insertar planes básicos si no existen
INSERT INTO plans (saas, name, description, monthly_price, yearly_price, features)
SELECT 'CRM', 'Plan Básico CRM', 'Plan básico para CRM', 29.99, 299.99, ARRAY['contacts', 'basic_reports']
WHERE NOT EXISTS (SELECT 1 FROM plans WHERE name = 'Plan Básico CRM' AND saas = 'CRM');

INSERT INTO plans (saas, name, description, monthly_price, yearly_price, features)
SELECT 'ERP', 'Plan Básico ERP', 'Plan básico para ERP', 49.99, 499.99, ARRAY['inventory', 'accounting']
WHERE NOT EXISTS (SELECT 1 FROM plans WHERE name = 'Plan Básico ERP' AND saas = 'ERP');

INSERT INTO plans (saas, name, description, monthly_price, yearly_price, features)
SELECT 'ECOMMERCE', 'Plan Básico E-commerce', 'Plan básico para E-commerce', 39.99, 399.99, ARRAY['products', 'orders']
WHERE NOT EXISTS (SELECT 1 FROM plans WHERE name = 'Plan Básico E-commerce' AND saas = 'ECOMMERCE');

-- Insertar roles de sistema si no existen
INSERT INTO roles (name, description, permissions, is_system)
SELECT 'super_admin', 'Administrador del sistema', ARRAY['*'], TRUE
WHERE NOT EXISTS (SELECT 1 FROM roles WHERE name = 'super_admin' AND is_system = TRUE);

INSERT INTO roles (name, description, permissions, is_system)
SELECT 'admin', 'Administrador de tenant', ARRAY['users:read', 'users:write', 'roles:read'], TRUE
WHERE NOT EXISTS (SELECT 1 FROM roles WHERE name = 'admin' AND is_system = TRUE);

INSERT INTO roles (name, description, permissions, is_system)
SELECT 'user', 'Usuario básico', ARRAY['profile:read', 'profile:write'], TRUE
WHERE NOT EXISTS (SELECT 1 FROM roles WHERE name = 'user' AND is_system = TRUE);

-- =================================================
-- 10. MENSAJE DE CONFIRMACIÓN
-- =================================================

DO $$ 
BEGIN 
    RAISE NOTICE 'Migración completada exitosamente. Todas las tablas, índices y datos iniciales están listos.';
END $$; 