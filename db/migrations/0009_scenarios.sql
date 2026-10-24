-- Scenarios (internal/shared/scenario): a simulation may run one, and a
-- scenario's faults are scoped to the simulation running it.
ALTER TABLE simulations ADD COLUMN scenario TEXT NOT NULL DEFAULT '';
ALTER TABLE fault_config ADD COLUMN simulation_id TEXT NOT NULL DEFAULT '';
