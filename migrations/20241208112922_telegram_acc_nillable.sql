-- Modify "telegram_accounts" table
ALTER TABLE "telegram_accounts" ALTER COLUMN "code" DROP NOT NULL, ALTER COLUMN "code_at" DROP NOT NULL, ALTER COLUMN "session" DROP NOT NULL;
