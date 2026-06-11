ALTER TABLE garage_vehicles ADD COLUMN mileage INTEGER NOT NULL DEFAULT 0;

CREATE TABLE garage_service_items (
    id                   UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    vehicle_id           UUID        NOT NULL REFERENCES garage_vehicles(id) ON DELETE CASCADE,
    kind                 TEXT        NOT NULL,            -- catalog key: oil, chain_lube, ... or 'custom'
    title                TEXT        NOT NULL DEFAULT '', -- display name (for custom / renamed items)
    interval_km          INTEGER     NOT NULL DEFAULT 0,  -- service interval from the manual
    last_service_mileage INTEGER     NOT NULL DEFAULT 0,  -- odometer reading at last reset
    last_service_at      TIMESTAMPTZ,                     -- date of last reset (for display)
    times_done           INTEGER     NOT NULL DEFAULT 0,  -- how many times serviced
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_garage_service_items_vehicle ON garage_service_items(vehicle_id);
