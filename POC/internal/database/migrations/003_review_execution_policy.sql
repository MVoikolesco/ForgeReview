ALTER TABLE review_policies
ADD COLUMN review_final_retries INTEGER NOT NULL DEFAULT 5;

ALTER TABLE review_policies
ADD COLUMN publish_manual_reviews INTEGER NOT NULL DEFAULT 0;

ALTER TABLE review_policies
ADD COLUMN allow_autonomous_rejection INTEGER NOT NULL DEFAULT 0;
