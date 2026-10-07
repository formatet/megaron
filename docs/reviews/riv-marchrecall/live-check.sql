BEGIN READ ONLY;
SELECT current_database(), current_timestamp;
SELECT count(*) AS total,
       count(*) FILTER (WHERE processed_at IS NULL AND failed_at IS NULL) AS pending,
       count(*) FILTER (WHERE processed_at IS NOT NULL) AS processed,
       count(*) FILTER (WHERE failed_at IS NOT NULL) AS failed
FROM scheduled_events WHERE event_type = 'MarchRecall';
COMMIT;
