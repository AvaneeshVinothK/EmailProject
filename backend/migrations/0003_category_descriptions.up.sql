-- 0003_category_descriptions.up.sql
ALTER TABLE categories ADD COLUMN description TEXT NOT NULL DEFAULT '';

UPDATE categories SET description = CASE name
    WHEN 'confirmation' THEN 'Automated acknowledgement that a job application was received or submitted. No action is required from the candidate.'
    WHEN 'next_steps' THEN 'Follow-up about an existing application that asks the candidate to take an action, such as providing availability, completing forms, or sending documents.'
    WHEN 'recruiter_reach_out' THEN 'Unsolicited outreach from a recruiter, sourcer, or hiring manager about a role the candidate has not applied to.'
    WHEN 'online_assessment' THEN 'Invitation, reminder, or result for a coding challenge, take-home assignment, or online assessment (e.g. HackerRank, CodeSignal).'
    WHEN 'interview' THEN 'Invitation to schedule, confirmation of, or update about an interview, including phone screens, technical rounds, and onsites.'
END;

INSERT INTO categories (name, description) VALUES
    ('other', 'Any email that does not clearly fit another category, including emails unrelated to a job search such as newsletters, promotions, receipts, and personal messages.');

ALTER TABLE categories ALTER COLUMN description DROP DEFAULT;
