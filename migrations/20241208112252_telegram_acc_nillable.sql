-- Modify "telegram_accounts" table
ALTER TABLE "telegram_accounts" DROP COLUMN "data", ALTER COLUMN "state" SET DEFAULT 'New';
