-- Temporary demo seed: a few example points per category across several cities.
-- These rows have created_by = NULL; they will be replaced by real, admin-managed
-- data once the admin panel lands (the down migration removes all author-less rows).
--
-- Each of the 20 categories is placed in every city, offset by a per-category delta
-- so markers of the same city don't stack. ~80 points total.
INSERT INTO points (created_by, category, name, description, lat, lon, address, phone, website, hours, photos)
SELECT
    NULL,
    c.category,
    c.label || ' · ' || ct.city,
    'Sample ' || c.label || ' in ' || ct.city,
    ct.lat + ((c.idx % 5) - 2) * 0.03,
    ct.lon + ((c.idx / 5) - 2) * 0.03,
    ct.city,
    '+7 495 555-' || lpad(c.idx::text, 2, '0') || '-00',
    'https://example.com/' || c.category,
    '09:00–21:00',
    '[]'::jsonb
FROM (VALUES
    (0,  'towing',        'Towing'),
    (1,  'tire_service',  'Tire service'),
    (2,  'service',       'Service'),
    (3,  'dealer',        'Dealer'),
    (4,  'gear',          'Gear'),
    (5,  'wash',          'Wash'),
    (6,  'bar',           'Biker bar'),
    (7,  'scenic',        'Scenic spot'),
    (8,  'racetrack',     'Racetrack'),
    (9,  'gymkhana',      'Gymkhana'),
    (10, 'gas_station',   'Gas station'),
    (11, 'parking',       'Parking'),
    (12, 'camping',       'Camping'),
    (13, 'cafe',          'Cafe'),
    (14, 'riding_school', 'Riding school'),
    (15, 'rental',        'Rental'),
    (16, 'parts',         'Parts'),
    (17, 'tuning',        'Tuning'),
    (18, 'police',        'Police / cameras'),
    (19, 'hazard',        'Hazard')
) AS c(idx, category, label)
CROSS JOIN (VALUES
    ('Moscow',           55.75, 37.62),
    ('Saint Petersburg', 59.94, 30.31),
    ('Kazan',            55.79, 49.12),
    ('Sochi',            43.59, 39.72)
) AS ct(city, lat, lon);
