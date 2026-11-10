-- Seeds the observations table from ~/observed_from.txt (as of 2026-11-10),
-- the prospect.completed events in the store, and where the observatories
-- are now. Run it once, with monitor stopped, after creating the table:
--
--   go run ./cmd/migrate_db
--   sqlite3 ~/.replicant/replicant.db < seed_observations.sql
--
-- Directions for the three rows under way are worked out here the way the
-- gaze task does: from the mean position of the relay stars with an entry
-- point, out through the star.

BEGIN IMMEDIATE;

-- Observed from before gaze kept notes (or, for SOL, never to be used:
-- ridiculously far away). Who observed and what they found isn't known.
WITH v(star) AS (VALUES ('SOL'), ('SYLSHENOS'), ('OTHYONAL'), ('SYRIREON'), ('ALWRENAN'), ('MIRETH'), ('WYNUS'), ('PARBEA')),
	t(now) AS (SELECT CAST(unixepoch('subsec') * 1000 AS INTEGER))
INSERT INTO observations (star, status, created_at, updated_at)
SELECT star, 'success', now, now FROM v, t;

-- Observed from, with what prospect.completed reported.
WITH v(star, device, found) AS (VALUES
		('INTERRANAE', '68D1D1CA', 'LORKOROTH,ALKANOTH,KELFERI,CORQUELAI,SELLOREL'),
		('CORBRENEUS', '4F7B4751', 'KELRELAR,ISTERRELAS,KYRZHBIPOS'),
		('NELRETHIS', '68D1D1CA', 'ISODORUR,IREYAFUR,SYRTELOR,NARBRENOTH,AELVENINT,DRAWYNIA')),
	t(now) AS (SELECT CAST(unixepoch('subsec') * 1000 AS INTEGER))
INSERT INTO observations (star, device_code, status, stars_found, created_at, updated_at)
SELECT star, device, 'success', found, now, now FROM v, t;

-- Refused as too dense (ledger notes: MUKONDO ratio=0.895, GALVALOS
-- ratio=0.955, XANIARNOTH ratio=0.716). The devices are the ones the
-- compact events show there.
WITH v(star, device) AS (VALUES ('MUKONDO', 'E5323D8E'), ('GALVALOS', '4F7B4751'), ('XANIARNOTH', '4F7B4751')),
	t(now) AS (SELECT CAST(unixepoch('subsec') * 1000 AS INTEGER))
INSERT INTO observations (star, device_code, status, created_at, updated_at)
SELECT star, device, 'too_dense', now, now FROM v, t;

-- Under way: E5323D8E has been prospecting from HALNULYN since
-- 2026-11-10T10:59+11:00; 68D1D1CA (MINKENETH) and 4F7B4751 (TEPHRAHA) are
-- unfurled at the entry point, waiting to prospect. Their gaze tasks will
-- prospect as soon as monitor starts.
WITH centre(cx, cy, cz) AS (
		SELECT avg(x), avg(y), avg(z) FROM stars
		WHERE designation IN (SELECT star FROM relay_stars) AND entry_point IS NOT NULL),
	v(star, device, status) AS (VALUES
		('HALNULYN', 'E5323D8E', 'prospecting'),
		('MINKENETH', '68D1D1CA', 'not_yet'),
		('TEPHRAHA', '4F7B4751', 'not_yet')),
	t(now) AS (SELECT CAST(unixepoch('subsec') * 1000 AS INTEGER))
INSERT INTO observations (star, dir_x, dir_y, dir_z, device_code, status, created_at, updated_at)
SELECT v.star, s.x - cx, s.y - cy, s.z - cz, v.device, v.status, now, now
FROM v JOIN stars s ON s.designation = v.star, centre, t;

SELECT star, device_code, status, round(dir_x, 2), round(dir_y, 2), round(dir_z, 2), stars_found
FROM observations ORDER BY status, star;

COMMIT;
