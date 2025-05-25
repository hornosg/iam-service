-- +migrate Up
ALTER TABLE tenants ADD COLUMN features JSONB DEFAULT '{"friends_family": false, "premium_analytics": false}';

-- Crear índice para búsquedas eficientes por features específicos
CREATE INDEX IF NOT EXISTS idx_tenants_features_friends_family ON tenants USING GIN ((features->'friends_family')) WHERE (features->>'friends_family')::boolean = true;
CREATE INDEX IF NOT EXISTS idx_tenants_features_premium_analytics ON tenants USING GIN ((features->'premium_analytics')) WHERE (features->>'premium_analytics')::boolean = true;

-- Actualizar registros existentes para que tengan el formato correcto de features
UPDATE tenants SET features = '{"friends_family": false, "premium_analytics": false}' WHERE features IS NULL;

-- +migrate Down
DROP INDEX IF EXISTS idx_tenants_features_friends_family;
DROP INDEX IF EXISTS idx_tenants_features_premium_analytics;
ALTER TABLE tenants DROP COLUMN features; 