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

-- Stars we know about. A star's position never changes. The counts are NULL
-- until the system has been scanned.
CREATE TABLE stars (
	designation TEXT PRIMARY KEY,
	x REAL NOT NULL,
	y REAL NOT NULL,
	z REAL NOT NULL,
	entry_point TEXT,                -- NULL until /stars or a system_scan tells us
	planet_count INTEGER,
	planets_scanned INTEGER,
	moon_count INTEGER,
	moons_scanned INTEGER,
	is_fully_scanned INTEGER AS (
		planets_scanned = planet_count AND moons_scanned = moon_count
	) STORED
) STRICT;

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
	arrives_at INTEGER,              -- travel.final_arrives_at, while travelling
	raw TEXT NOT NULL                -- the whole /devices response, as JSON
) STRICT;

CREATE INDEX devices_by_location ON devices (location);
CREATE INDEX devices_by_star ON devices (star, device_type);
CREATE INDEX devices_by_type ON devices (device_type, status);

-- Stars with a relaying (not stowed) ftl_relay: the relay mesh.
CREATE VIEW relay_stars AS
	SELECT DISTINCT star FROM devices
	WHERE device_type = 'ftl_relay' AND status = 'relaying';
