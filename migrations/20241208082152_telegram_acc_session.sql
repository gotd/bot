-- Modify "telegram_accounts" table
ALTER TABLE "telegram_accounts" ADD COLUMN "session" bytea NOT NULL;
