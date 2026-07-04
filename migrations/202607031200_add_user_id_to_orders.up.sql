-- Per-user order ownership (HANDOFF goal 3: make users meaningful).
--
-- Backfill story for existing POC rows: they are throwaway data, so they
-- are claimed by a sentinel owner via the column DEFAULT at ADD COLUMN
-- time (single pass, no table rewrite pain at this size). The DEFAULT is
-- then dropped so every future INSERT must supply the real subject
-- explicitly — forgetting to pass user_id becomes a hard error instead
-- of silently mis-owned rows.
ALTER TABLE orders
    ADD COLUMN user_id TEXT NOT NULL DEFAULT 'legacy:pre-auth';

ALTER TABLE orders
    ALTER COLUMN user_id DROP DEFAULT;

-- Every read path is now owner-scoped (WHERE user_id = ...), so this
-- index backs effectively all queries.
CREATE INDEX idx_orders_user_id ON orders (user_id);
