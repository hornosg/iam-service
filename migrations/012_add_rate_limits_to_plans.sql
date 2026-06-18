-- Migration: rate_limits por plan (ADR-003 D4) — habilita pricing por volumen.
-- Columna JSONB aditiva: matriz feature → regla {algorithm, limit/window | rate/burst}.
-- Algoritmo por celda (plan × feature): gcra (ritmo constante, FREE) o
-- sliding_window_counter (preciso, AI/infra/planes pagos). Los servicios cachean tier→reglas.

ALTER TABLE plans ADD COLUMN IF NOT EXISTS rate_limits JSONB NOT NULL DEFAULT '{}'::jsonb;
CREATE INDEX IF NOT EXISTS idx_plans_rate_limits ON plans USING GIN (rate_limits);

-- Seeds por tier (valores iniciales, ajustables por admin). Catálogo de features del ADR.
UPDATE plans SET rate_limits = '{
  "ai.bi_query":      {"algorithm":"gcra","rate":"10/s","burst":0},
  "ai.document_ocr":  {"algorithm":"sliding_window_counter","limit":20,"window":"1h"},
  "ai.voice":         {"algorithm":"sliding_window_counter","limit":10,"window":"1h"},
  "ai.categorization":{"algorithm":"sliding_window_counter","limit":100,"window":"1h"},
  "webdata.scrape":   {"algorithm":"sliding_window_counter","limit":5,"window":"1m"},
  "pim.bulk_import":  {"algorithm":"sliding_window_counter","limit":2,"window":"1h"}
}'::jsonb WHERE type = 'FREE';

UPDATE plans SET rate_limits = '{
  "ai.bi_query":      {"algorithm":"gcra","rate":"30/s","burst":10},
  "ai.document_ocr":  {"algorithm":"sliding_window_counter","limit":80,"window":"1h"},
  "ai.voice":         {"algorithm":"sliding_window_counter","limit":40,"window":"1h"},
  "ai.categorization":{"algorithm":"sliding_window_counter","limit":400,"window":"1h"},
  "webdata.scrape":   {"algorithm":"sliding_window_counter","limit":20,"window":"1m"},
  "pim.bulk_import":  {"algorithm":"sliding_window_counter","limit":8,"window":"1h"}
}'::jsonb WHERE type = 'BASIC';

UPDATE plans SET rate_limits = '{
  "ai.bi_query":      {"algorithm":"sliding_window_counter","limit":100,"window":"1s"},
  "ai.document_ocr":  {"algorithm":"sliding_window_counter","limit":200,"window":"1h"},
  "ai.voice":         {"algorithm":"sliding_window_counter","limit":100,"window":"1h"},
  "ai.categorization":{"algorithm":"sliding_window_counter","limit":1000,"window":"1h"},
  "webdata.scrape":   {"algorithm":"sliding_window_counter","limit":60,"window":"1m"},
  "pim.bulk_import":  {"algorithm":"sliding_window_counter","limit":20,"window":"1h"}
}'::jsonb WHERE type = 'PREMIUM';

UPDATE plans SET rate_limits = '{
  "ai.bi_query":      {"algorithm":"sliding_window_counter","limit":500,"window":"1s"},
  "ai.document_ocr":  {"algorithm":"sliding_window_counter","limit":2000,"window":"1h"},
  "ai.voice":         {"algorithm":"sliding_window_counter","limit":1000,"window":"1h"},
  "ai.categorization":{"algorithm":"sliding_window_counter","limit":10000,"window":"1h"},
  "webdata.scrape":   {"algorithm":"sliding_window_counter","limit":300,"window":"1m"},
  "pim.bulk_import":  {"algorithm":"sliding_window_counter","limit":100,"window":"1h"}
}'::jsonb WHERE type = 'ENTERPRISE';
