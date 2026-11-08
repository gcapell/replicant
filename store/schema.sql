-- The database schema. Edit this file to change it: store.Open compares it with
-- the database and adds, rebuilds or drops tables to match (see migrate.go).
--
-- Conventions:
--   - Plain CREATE statements (no IF NOT EXISTS); the migrator decides what to
--     create.
--   - STRICT tables, so a wrongly typed value is an error, not a surprise.
--   - Booleans are INTEGER 0/1 (SQLite has no bool type; TRUE and FALSE are
--     just 1 and 0).
--   - Times are INTEGER Unix milliseconds (time.UnixMilli). To read one:
--     datetime(t / 1000.0, 'unixepoch', 'subsec').
--   - A new NOT NULL column needs a DEFAULT, or rebuilding a table that
--     already has rows fails.
--   - Renaming a column or table loses its data (it looks like a drop and an
--     add).

-- Every event we've received from the stream, in the order we got it. An
-- event is recorded in the same transaction that applies it, so the last row
-- is where to resume the stream from. (Event IDs, like "1794115235797-0",
-- don't sort as text, hence seq.)
CREATE TABLE events (
	seq INTEGER PRIMARY KEY,         -- order received
	event_id TEXT NOT NULL UNIQUE,
	version INTEGER NOT NULL,
	category TEXT NOT NULL,
	event TEXT NOT NULL,             -- e.g. travel.arrived
	created_at INTEGER NOT NULL,
	device_code TEXT,
	device_type TEXT,
	replicant_code TEXT,
	star TEXT,
	location TEXT,
	payload TEXT NOT NULL            -- the event's payload, as JSON
) STRICT;

CREATE INDEX events_by_device ON events (device_code, created_at);
CREATE INDEX events_by_event ON events (event, created_at);

-- Stars we know about. A star's position never changes. The columns after
-- entry_point come from /locations/STAR, which only answers for a system we
-- have a presence in, so they're NULL until we've been able to read it.
CREATE TABLE stars (
	designation TEXT PRIMARY KEY,
	x REAL NOT NULL,
	y REAL NOT NULL,
	z REAL NOT NULL,
	entry_point TEXT,                -- NULL until /stars or a system_scan tells us
	planet_count INTEGER,            -- planets_total
	planets_scanned INTEGER,
	moon_count INTEGER,              -- moons_total
	moons_scanned INTEGER,
	is_fully_scanned INTEGER AS (
		planets_scanned = planet_count AND moons_scanned = moon_count
	) STORED,
	life_detected INTEGER,
	mining_bonus_pct REAL,
	system_tags TEXT                 -- comma-separated, e.g. resource_rich,green
) STRICT;

-- Planets (and later moons and other places), from /locations/STAR.
CREATE TABLE locations (
	designation TEXT PRIMARY KEY,
	star TEXT NOT NULL AS (substr(designation, 1, instr(designation, '-') - 1)) STORED,
	type TEXT,                       -- e.g. dwarf, terrestrial, ice giant
	in_habitable_zone INTEGER NOT NULL,
	life_stage TEXT,                 -- e.g. none, intelligent
	moon_count INTEGER,
	scanned INTEGER NOT NULL
) STRICT;

CREATE INDEX locations_by_star ON locations (star);

-- Asteroid belts, from /locations/STAR. The resource columns hold levels
-- (low, moderate, high), not quantities.
CREATE TABLE belts (
	designation TEXT PRIMARY KEY,    -- e.g. OTHYONAL-BELT-1
	star TEXT NOT NULL AS (substr(designation, 1, instr(designation, '-') - 1)) STORED,
	density TEXT,
	carbon TEXT,
	conductive TEXT,
	rares TEXT,
	silicates TEXT,
	structural TEXT,
	volatiles TEXT
) STRICT;

CREATE INDEX belts_by_star ON belts (star);

-- Resources lying at a location (planets, moons and belts), from /inventory.
-- A location holding nothing has no row.
CREATE TABLE inventory (
	designation TEXT PRIMARY KEY,
	star TEXT NOT NULL AS (substr(designation, 1, instr(designation, '-') - 1)) STORED,
	carbon INTEGER NOT NULL DEFAULT 0,
	conductive INTEGER NOT NULL DEFAULT 0,
	rares INTEGER NOT NULL DEFAULT 0,
	silicates INTEGER NOT NULL DEFAULT 0,
	structural INTEGER NOT NULL DEFAULT 0,
	volatiles INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE INDEX inventory_by_star ON inventory (star);

-- Our devices, as last seen.
CREATE TABLE devices (
	device_code TEXT PRIMARY KEY,
	device_type TEXT NOT NULL,
	owner TEXT,                      -- replicant_code
	status TEXT NOT NULL,
	location TEXT,
	star TEXT AS (                   -- the part of location before the first '-'
		CASE WHEN instr(location, '-') > 0
			THEN substr(location, 1, instr(location, '-') - 1)
			ELSE location END
	) STORED,
	attached_to TEXT,                -- attached_to_device_code
	stowed_in TEXT,                  -- stowed_in_device_code
	controller TEXT,                 -- controller_device_code
	destination TEXT,                -- travel.final_destination, while travelling
	arrives_at INTEGER               -- travel.final_arrives_at, while travelling
) STRICT;

CREATE INDEX devices_by_location ON devices (location);
CREATE INDEX devices_by_star ON devices (star, device_type);
CREATE INDEX devices_by_type ON devices (device_type, status);

-- Stars with a relaying (not stowed) ftl_relay: the relay mesh.
CREATE VIEW relay_stars AS
	SELECT DISTINCT star FROM devices
	WHERE device_type = 'ftl_relay' AND status = 'relaying';
