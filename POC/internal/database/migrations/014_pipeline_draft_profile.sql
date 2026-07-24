ALTER TABLE pipeline_versions ADD COLUMN target_profile_id INTEGER REFERENCES review_profiles(id);

UPDATE pipeline_versions
SET target_profile_id=(
    SELECT profile_id FROM pipeline_definitions
    WHERE pipeline_definitions.id=pipeline_versions.pipeline_definition_id
);
