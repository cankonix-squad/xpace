WITH ranked_participants AS (
  SELECT id,
         ROW_NUMBER() OVER (
           PARTITION BY meeting_id, user_id
           ORDER BY created_at DESC, id DESC
         ) AS active_rank
  FROM meeting_participants
  WHERE user_id IS NOT NULL
    AND status NOT IN ('LEFT', 'REMOVED')
)
UPDATE meeting_participants AS participant
SET status = 'LEFT',
    left_at = COALESCE(participant.left_at, NOW())
FROM ranked_participants AS ranked
WHERE participant.id = ranked.id
  AND ranked.active_rank > 1;

CREATE UNIQUE INDEX IF NOT EXISTS meeting_participants_active_user_unique
  ON meeting_participants (meeting_id, user_id)
  WHERE user_id IS NOT NULL
    AND status NOT IN ('LEFT', 'REMOVED');
