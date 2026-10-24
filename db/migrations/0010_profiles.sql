-- Simulation profiles (internal/shared/sim/profile.go): the effective
-- profile a simulation runs with ('' = none).
ALTER TABLE simulations ADD COLUMN profile TEXT NOT NULL DEFAULT '';
