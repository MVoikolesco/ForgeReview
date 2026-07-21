ALTER TABLE ai_connections
ADD COLUMN http_referer TEXT NOT NULL DEFAULT '';

ALTER TABLE ai_connections
ADD COLUMN app_title TEXT NOT NULL DEFAULT '';
