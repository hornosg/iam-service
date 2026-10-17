-- ACC-E04.T2: mudanza del schema de tenant-service/001 (incluidos comentarios).
-- Caso de uso: read/set/bootstrap con tenant conocido, después del gate de ADR-005.
-- No existe config pre-auth: iam_login no recibe grants ni policy de escape.
-- Molde 019: SET LOCAL app.tenant_id por transacción; ausencia/RESET falla cerrado.
-- account_app no tiene DDL; account_migrator crea tabla y policy (FORCE también al owner).
-- El GRANT forward es necesario en bases donde 018 ya se aplicó.

-- Migration: Create tenant_config table
-- Description: Tabla para almacenar configuraciones por tenant
-- Author: System
-- Date: 2026-02-03

-- ============================================
-- Create tenant_config table
-- ============================================

CREATE TABLE IF NOT EXISTS tenant_config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    config_key VARCHAR(100) NOT NULL,
    config_value VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now(),
    CONSTRAINT uq_tenant_config UNIQUE (tenant_id, config_key)
);

-- ============================================
-- Create indexes
-- ============================================

CREATE INDEX IF NOT EXISTS idx_tenant_config_tenant
    ON tenant_config (tenant_id);

CREATE INDEX IF NOT EXISTS idx_tenant_config_key
    ON tenant_config (config_key);

-- ============================================
-- Comments
-- ============================================

COMMENT ON TABLE tenant_config IS 'Configuraciones por tenant (policies, settings, keys)';
COMMENT ON COLUMN tenant_config.id IS 'Identificador único de la configuración';
COMMENT ON COLUMN tenant_config.tenant_id IS 'Identificador del tenant';
COMMENT ON COLUMN tenant_config.config_key IS 'Clave de configuración (namespaced, ej: catalog.stock_policy)';
COMMENT ON COLUMN tenant_config.config_value IS 'Valor de la configuración (string)';
COMMENT ON COLUMN tenant_config.created_at IS 'Fecha de creación';
COMMENT ON COLUMN tenant_config.updated_at IS 'Fecha de última actualización';

-- Re-aplicable: ENABLE/FORCE y GRANT idempotentes; policy se reemplaza antes de crear.
ALTER TABLE tenant_config ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_config FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON tenant_config;
CREATE POLICY tenant_isolation ON tenant_config
  USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON tenant_config TO account_app;
