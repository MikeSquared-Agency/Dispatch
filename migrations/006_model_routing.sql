-- 006_model_routing.sql
-- Adds model-aware cost routing fields to swarm_tasks.

ALTER TABLE swarm_tasks
    ADD COLUMN labels TEXT[] DEFAULT '{}',
    ADD COLUMN file_patterns TEXT[] DEFAULT '{}',
    ADD COLUMN one_way_door BOOLEAN DEFAULT false,
    ADD COLUMN recommended_model TEXT,
    ADD COLUMN model_tier TEXT,
    ADD COLUMN routing_method TEXT,
    ADD COLUMN runtime TEXT;
