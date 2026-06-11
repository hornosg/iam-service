ALTER TABLE tenants ADD COLUMN IF NOT EXISTS namespace VARCHAR(50) NOT NULL DEFAULT 'mc';

CREATE INDEX IF NOT EXISTS idx_tenants_namespace ON tenants(namespace);

ALTER TABLE tenants DROP CONSTRAINT IF EXISTS tenants_slug_key;
ALTER TABLE tenants ADD CONSTRAINT tenants_namespace_slug_unique UNIQUE (namespace, slug);

UPDATE tenants SET namespace = 'mc' WHERE namespace IS NULL OR namespace = '';
