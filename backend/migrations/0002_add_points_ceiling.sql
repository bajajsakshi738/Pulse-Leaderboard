-- 0002_add_points_ceiling.sql
-- Adds an upper bound to score submissions so a single row can never
-- be worth an unbounded number of points. If you already ran
-- 0001_init.sql against your database, run this migration too;
-- 0001_init.sql alone won't retroactively alter an existing table.

ALTER TABLE scores DROP CONSTRAINT IF EXISTS scores_points_check;
ALTER TABLE scores ADD CONSTRAINT scores_points_check CHECK (points > 0 AND points <= 10000);
