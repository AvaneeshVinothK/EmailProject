-- 0003_category_descriptions.down.sql
-- Emails classified as 'other' become unclassified and are picked up again on the next fetch.
DELETE FROM classifications
WHERE category_id = (SELECT id FROM categories WHERE name = 'other');

DELETE FROM categories WHERE name = 'other';

ALTER TABLE categories DROP COLUMN description;
