-- Migration: Create plans table
-- Description: Tabla para almacenar planes del sistema

CREATE TABLE IF NOT EXISTS plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT NOT NULL,
    type VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
    max_users INTEGER NOT NULL,
    price_month DECIMAL(10,2) NOT NULL DEFAULT 0,
    price_year DECIMAL(10,2) NOT NULL DEFAULT 0,
    features TEXT[], -- Array de características
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT plans_type_check CHECK (type IN ('FREE', 'BASIC', 'PREMIUM', 'ENTERPRISE')),
    CONSTRAINT plans_status_check CHECK (status IN ('ACTIVE', 'INACTIVE', 'DEPRECATED')),
    CONSTRAINT plans_max_users_check CHECK (max_users >= -1), -- -1 para unlimited
    CONSTRAINT plans_price_check CHECK (price_month >= 0 AND price_year >= 0)
);

-- Indexes para optimizar consultas
CREATE INDEX IF NOT EXISTS idx_plans_type ON plans(type);
CREATE INDEX IF NOT EXISTS idx_plans_status ON plans(status);
CREATE INDEX IF NOT EXISTS idx_plans_name ON plans(name);
CREATE INDEX IF NOT EXISTS idx_plans_created_at ON plans(created_at);

-- Trigger para actualizar updated_at automáticamente
CREATE TRIGGER plans_update_updated_at 
    BEFORE UPDATE ON plans 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

-- Insertar planes por defecto
INSERT INTO plans (name, description, type, max_users, price_month, price_year, features) VALUES
('Free Plan', 'Perfect for getting started', 'FREE', 1, 0, 0, ARRAY['1 User', 'Basic Features', 'Community Support']),
('Basic Plan', 'For small teams', 'BASIC', 10, 9.99, 99.99, ARRAY['Up to 10 Users', 'Standard Features', 'Email Support']),
('Premium Plan', 'For growing businesses', 'PREMIUM', 100, 29.99, 299.99, ARRAY['Up to 100 Users', 'Advanced Features', 'Priority Support']),
('Enterprise Plan', 'For large organizations', 'ENTERPRISE', -1, 99.99, 999.99, ARRAY['Unlimited Users', 'All Features', '24/7 Support', 'Custom Integration'])
ON CONFLICT (name) DO NOTHING; 