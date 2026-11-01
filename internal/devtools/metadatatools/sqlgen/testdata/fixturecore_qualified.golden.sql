CREATE TABLE "erpora_expected"."fixturecore_branches" (
    "id" uuid NOT NULL,
    "version" bigint NOT NULL,
    "created_at" timestamp with time zone NOT NULL,
    "updated_at" timestamp with time zone NOT NULL,
    "created_by" uuid NOT NULL,
    "updated_by" uuid NOT NULL,
    "code" text NOT NULL,
    "name" text NOT NULL,
    CONSTRAINT "fixturecore_branches_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "fixturecore_branches_code_key" UNIQUE ("code"),
    CONSTRAINT "fixturecore_branches_version_check" CHECK ("version" >= 1),
    CONSTRAINT "fixturecore_branches_code_check" CHECK (length("code") <= 16),
    CONSTRAINT "fixturecore_branches_name_check" CHECK (length("name") <= 120)
);

CREATE TABLE "erpora_expected"."fixturecore_customers" (
    "id" uuid NOT NULL,
    "version" bigint NOT NULL,
    "created_at" timestamp with time zone NOT NULL,
    "updated_at" timestamp with time zone NOT NULL,
    "created_by" uuid NOT NULL,
    "updated_by" uuid NOT NULL,
    "billing_street" text,
    "billing_city" text,
    "billing_postal_code" text,
    "code" text NOT NULL,
    "name" text NOT NULL,
    "status" text DEFAULT 'active' NOT NULL,
    "contact_email" text,
    "contact_phone" text,
    "credit_limit" numeric(18,2) DEFAULT 0.00 NOT NULL,
    "blocked_reason" text,
    CONSTRAINT "fixturecore_customers_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "fixturecore_customers_code_key" UNIQUE ("code"),
    CONSTRAINT "fixturecore_customers_version_check" CHECK ("version" >= 1),
    CONSTRAINT "fixturecore_customers_billing_street_check" CHECK (length("billing_street") <= 200),
    CONSTRAINT "fixturecore_customers_billing_city_check" CHECK (length("billing_city") <= 100),
    CONSTRAINT "fixturecore_customers_billing_postal_code_check" CHECK (length("billing_postal_code") <= 10),
    CONSTRAINT "fixturecore_customers_code_check" CHECK (length("code") <= 32),
    CONSTRAINT "fixturecore_customers_name_check" CHECK (length("name") <= 200),
    CONSTRAINT "fixturecore_customers_status_check" CHECK ("status" IN ('active', 'inactive', 'blocked')),
    CONSTRAINT "fixturecore_customers_contact_email_check" CHECK (length("contact_email") <= 254),
    CONSTRAINT "fixturecore_customers_contact_phone_check" CHECK (length("contact_phone") <= 32),
    CONSTRAINT "fixturecore_customers_credit_limit_check" CHECK ("credit_limit" >= 0),
    CONSTRAINT "fixturecore_customers_blocked_reason_check" CHECK (length("blocked_reason") <= 500)
);

CREATE INDEX "fixturecore_customers_status_name_idx" ON "erpora_expected"."fixturecore_customers" ("status", "name");

