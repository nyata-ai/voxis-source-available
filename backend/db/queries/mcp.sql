-- name: SearchAllEntities :many
SELECT
  m.id::text AS id,
  'media'::text AS entity_type,
  COALESCE(NULLIF(TRIM(m.title), ''), m.filename, '') AS title,
  COALESCE(m.description, '') AS description,
  m.filename,
  m.status,
  m.created_at,
  COALESCE((
    SELECT t.id::text FROM transcriptions t
    WHERE t.media_id = m.id AND t.organization_id = m.organization_id AND t.status = 'completed'
    ORDER BY t.completed_at DESC NULLS LAST LIMIT 1
  ), '') AS latest_transcription_id
FROM media m
WHERE m.organization_id = @organization_id
  AND m.status != 'deleted'
  AND (
    m.filename ILIKE '%' || replace(replace(replace(@search::text, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\'
    OR COALESCE(m.title, '') ILIKE '%' || replace(replace(replace(@search::text, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\'
    OR COALESCE(m.description, '') ILIKE '%' || replace(replace(replace(@search::text, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\'
  )
ORDER BY m.created_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: CountSearchAllEntities :one
SELECT COUNT(*) FROM media m
WHERE m.organization_id = @organization_id
  AND m.status != 'deleted'
  AND (
    m.filename ILIKE '%' || replace(replace(replace(@search::text, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\'
    OR COALESCE(m.title, '') ILIKE '%' || replace(replace(replace(@search::text, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\'
    OR COALESCE(m.description, '') ILIKE '%' || replace(replace(replace(@search::text, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\'
  );

-- name: ListTranscriptionsWithoutSummaries :many
SELECT
  t.id::text AS id,
  'media'::text AS source_type,
  t.status,
  t.word_count,
  t.speaker_count,
  t.duration_seconds,
  t.created_at,
  COALESCE(NULLIF(TRIM(m.title), ''), m.filename, '') AS display_name,
  COALESCE(m.description, '') AS description
FROM transcriptions t
JOIN media m ON t.media_id = m.id AND m.status != 'deleted'
WHERE t.organization_id = @organization_id
  AND t.status = 'completed'
  AND NOT EXISTS (
    SELECT 1 FROM summaries s
    WHERE s.transcription_id = t.id AND s.status = 'completed'
      AND (@summary_type::text = '' OR s.summary_type = @summary_type)
  )
ORDER BY t.created_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: CreateMCPCollection :one
INSERT INTO mcp_collections (organization_id, name, description, created_by)
VALUES (@organization_id, @name, @description, @created_by)
RETURNING id::text, organization_id::text, name, description, created_by, created_at, updated_at;

-- name: ListMCPCollections :many
SELECT c.id::text AS id, c.organization_id::text AS organization_id, c.name, c.description,
  c.created_by, c.created_at, c.updated_at, COUNT(i.transcription_id)::int AS item_count
FROM mcp_collections c
LEFT JOIN mcp_collection_items i ON i.collection_id = c.id
WHERE c.organization_id = @organization_id
GROUP BY c.id
ORDER BY c.created_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: GetMCPCollection :one
SELECT c.id::text AS id, c.organization_id::text AS organization_id, c.name, c.description,
  c.created_by, c.created_at, c.updated_at, COUNT(i.transcription_id)::int AS item_count
FROM mcp_collections c
LEFT JOIN mcp_collection_items i ON i.collection_id = c.id
WHERE c.organization_id = @organization_id AND c.id = @collection_id
GROUP BY c.id;

-- name: AddMCPCollectionItem :exec
INSERT INTO mcp_collection_items (collection_id, transcription_id)
SELECT c.id, t.id
FROM mcp_collections c
JOIN transcriptions t ON t.id = @transcription_id
  AND t.organization_id = c.organization_id AND t.status != 'deleted'
WHERE c.organization_id = @organization_id AND c.id = @collection_id
ON CONFLICT (collection_id, transcription_id) DO NOTHING;

-- name: RemoveMCPCollectionItem :exec
DELETE FROM mcp_collection_items i
USING mcp_collections c
WHERE i.collection_id = c.id
  AND c.organization_id = @organization_id
  AND i.collection_id = @collection_id
  AND i.transcription_id = @transcription_id;

-- name: ListMCPCollectionItems :many
SELECT i.collection_id::text AS collection_id, i.transcription_id::text AS transcription_id, i.added_at
FROM mcp_collection_items i
JOIN mcp_collections c ON c.id = i.collection_id
JOIN transcriptions t ON t.id = i.transcription_id
WHERE c.organization_id = @organization_id AND i.collection_id = @collection_id
  AND t.organization_id = c.organization_id AND t.status != 'deleted'
ORDER BY i.added_at DESC
LIMIT @limit_val OFFSET @offset_val;
