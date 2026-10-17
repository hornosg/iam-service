-- ACC-E04.T8 down: restaura el default de 010 y el valor previo. Reversible sin
-- pérdida en el lab; aborta (fail-closed) si existiera ('mc', s) junto a
-- ('mercado-cercano', s), en lugar de fusionar tenants.

ALTER TABLE tenants ALTER COLUMN namespace SET DEFAULT 'mc';

-- Mismo escape transaccional que el up (FORCE RLS ata al owner).
SELECT set_config('app.is_system_admin', 'true', true);

UPDATE tenants SET namespace = 'mc'
 WHERE namespace = 'mercado-cercano';
