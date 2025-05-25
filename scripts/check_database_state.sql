-- Script para verificar el estado actual de la base de datos
-- Úsalo para ver qué tablas, índices y datos ya existen

\echo '================================================='
\echo 'ESTADO ACTUAL DE LA BASE DE DATOS IAM'
\echo '================================================='

-- 1. Verificar qué tablas existen
\echo ''
\echo '1. TABLAS EXISTENTES:'
\echo '--------------------'
SELECT table_name, table_type
FROM information_schema.tables 
WHERE table_schema = 'public' 
AND table_name IN ('plans', 'roles', 'tenants', 'users')
ORDER BY table_name;

-- 2. Verificar qué tipos personalizados existen
\echo ''
\echo '2. TIPOS ENUM EXISTENTES:'
\echo '------------------------'
SELECT typname, typtype
FROM pg_type 
WHERE typname IN ('saas_type')
ORDER BY typname;

-- 3. Verificar índices existentes
\echo ''
\echo '3. ÍNDICES EXISTENTES:'
\echo '---------------------'
SELECT schemaname, tablename, indexname
FROM pg_indexes 
WHERE schemaname = 'public'
AND tablename IN ('plans', 'roles', 'tenants', 'users')
ORDER BY tablename, indexname;

-- 4. Verificar triggers existentes
\echo ''
\echo '4. TRIGGERS EXISTENTES:'
\echo '----------------------'
SELECT trigger_name, event_object_table
FROM information_schema.triggers
WHERE event_object_schema = 'public'
AND event_object_table IN ('plans', 'roles', 'tenants', 'users')
ORDER BY event_object_table, trigger_name;

-- 5. Verificar funciones existentes
\echo ''
\echo '5. FUNCIONES EXISTENTES:'
\echo '-----------------------'
SELECT routine_name, routine_type
FROM information_schema.routines
WHERE routine_schema = 'public'
AND routine_name LIKE '%update%'
ORDER BY routine_name;

-- 6. Contar registros en cada tabla (si existen)
\echo ''
\echo '6. CANTIDAD DE REGISTROS:'
\echo '------------------------'

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'plans' AND table_schema = 'public') THEN
        RAISE NOTICE 'plans: % registros', (SELECT COUNT(*) FROM plans);
    ELSE
        RAISE NOTICE 'plans: tabla no existe';
    END IF;
    
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'roles' AND table_schema = 'public') THEN
        RAISE NOTICE 'roles: % registros', (SELECT COUNT(*) FROM roles);
    ELSE
        RAISE NOTICE 'roles: tabla no existe';
    END IF;
    
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'tenants' AND table_schema = 'public') THEN
        RAISE NOTICE 'tenants: % registros', (SELECT COUNT(*) FROM tenants);
    ELSE
        RAISE NOTICE 'tenants: tabla no existe';
    END IF;
    
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users' AND table_schema = 'public') THEN
        RAISE NOTICE 'users: % registros', (SELECT COUNT(*) FROM users);
    ELSE
        RAISE NOTICE 'users: tabla no existe';
    END IF;
END $$;

-- 7. Verificar datos de ejemplo en cada tabla (primeros 3 registros)
\echo ''
\echo '7. DATOS DE EJEMPLO:'
\echo '-------------------'

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'plans' AND table_schema = 'public') THEN
        RAISE NOTICE 'Primeros planes existentes:';
        PERFORM name, saas, monthly_price FROM plans LIMIT 3;
    END IF;
    
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'roles' AND table_schema = 'public') THEN
        RAISE NOTICE 'Primeros roles existentes:';
        PERFORM name, is_system FROM roles LIMIT 3;
    END IF;
END $$;

\echo ''
\echo '================================================='
\echo 'VERIFICACIÓN COMPLETADA'
\echo '=================================================' 