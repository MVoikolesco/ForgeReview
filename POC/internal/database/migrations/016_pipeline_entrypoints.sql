ALTER TABLE pipeline_version_triggers ADD COLUMN target_stage_key TEXT NOT NULL DEFAULT '';
ALTER TABLE pipeline_version_triggers ADD COLUMN config_json TEXT NOT NULL DEFAULT '{}';

UPDATE pipeline_version_triggers
SET target_stage_key = COALESCE((
    SELECT ps.stage_key
    FROM pipeline_stages ps
    JOIN stage_types st ON st.id = ps.stage_type_id
    WHERE ps.pipeline_version_id = pipeline_version_triggers.pipeline_version_id
      AND st.executor_key = 'preparation'
    ORDER BY ps.position
    LIMIT 1
), '')
WHERE target_stage_key = '';

UPDATE pipeline_version_triggers
SET config_json = CASE trigger_source
    WHEN 'webhook' THEN '{"x":-360,"y":-145}'
    WHEN 'api' THEN '{"x":-360,"y":0}'
    ELSE '{"x":-360,"y":145}'
END
WHERE config_json = '{}';
