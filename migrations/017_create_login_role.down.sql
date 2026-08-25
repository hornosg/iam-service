-- Migration: rollback del rol de login (iam_login)
-- Description: revierte 017 — revoca los grants por columnas y dropea el rol.
-- Idempotente: REVOKE es no-op si no existe el grant; DROP ROLE IF EXISTS.

-- 1. Revocar SELECT por columnas (debe listar las mismas 8 de 017).
REVOKE SELECT (id, email, password_hash, tenant_id, role_id, status, provider, federated_id)
  ON users FROM iam_login;

-- 2. Revocar acceso a schema y DB.
REVOKE USAGE ON SCHEMA public FROM iam_login;
REVOKE CONNECT ON DATABASE iam_db FROM iam_login;

-- 3. Dropear el rol. Sin objetos propios ni grants => cae limpio.
DROP ROLE IF EXISTS iam_login;