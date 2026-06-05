-- Remove the demo seed (all author-less points).
DELETE FROM points WHERE created_by IS NULL;
