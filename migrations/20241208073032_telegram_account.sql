-- Create "telegram_accounts" table
CREATE TABLE "telegram_accounts" ("id" character varying NOT NULL, "code" character varying NOT NULL, "code_at" timestamptz NOT NULL, "data" bytea NOT NULL, "state" character varying NOT NULL, "status" character varying NOT NULL, PRIMARY KEY ("id"));
