CREATE TABLE "fixturecore_branches" (
    "id" uuid NOT NULL,
    "version" bigint DEFAULT 1 NOT NULL,
    "created_at" timestamp with time zone DEFAULT now() NOT NULL,
    "updated_at" timestamp with time zone DEFAULT now() NOT NULL,
    "created_by" uuid,
    "updated_by" uuid,
    "code" text NOT NULL,
    "name" text NOT NULL,
    CONSTRAINT "fixturecore_branches_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "fixturecore_branches_code_key" UNIQUE ("code"),
    CONSTRAINT "fixturecore_branches_version_check" CHECK ("version" >= 1),
    CONSTRAINT "fixturecore_branches_code_check" CHECK (length("code") <= 16),
    CONSTRAINT "fixturecore_branches_name_check" CHECK (length("name") <= 120)
);

CREATE TABLE "fixturecore_customers" (
    "id" uuid NOT NULL,
    "version" bigint DEFAULT 1 NOT NULL,
    "created_at" timestamp with time zone DEFAULT now() NOT NULL,
    "updated_at" timestamp with time zone DEFAULT now() NOT NULL,
    "created_by" uuid,
    "updated_by" uuid,
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
    "phone" text,
    "phone_e164" text,
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
    CONSTRAINT "fixturecore_customers_blocked_reason_check" CHECK (length("blocked_reason") <= 500),
    CONSTRAINT "fixturecore_customers_phone_check" CHECK (length("phone") <= 32),
    CONSTRAINT "fixturecore_customers_phone_e164_check" CHECK (length("phone_e164") <= 20)
);

CREATE TABLE "fixturesales_invoice_lines" (
    "id" uuid NOT NULL,
    "version" bigint DEFAULT 1 NOT NULL,
    "created_at" timestamp with time zone DEFAULT now() NOT NULL,
    "updated_at" timestamp with time zone DEFAULT now() NOT NULL,
    "created_by" uuid,
    "updated_by" uuid,
    "line_no" bigint NOT NULL,
    "product_sku" text NOT NULL,
    "description" text NOT NULL,
    "quantity" numeric(18,3) NOT NULL,
    "unit_price" numeric(18,2) NOT NULL,
    "line_total" numeric(18,2) GENERATED ALWAYS AS (round(("quantity" * "unit_price"), 2)) STORED NOT NULL,
    "invoice_id" uuid NOT NULL,
    "product_id" uuid NOT NULL,
    CONSTRAINT "fixturesales_invoice_lines_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "fixturesales_invoice_lines_version_check" CHECK ("version" >= 1),
    CONSTRAINT "fixturesales_invoice_lines_line_no_check" CHECK ("line_no" >= 1),
    CONSTRAINT "fixturesales_invoice_lines_product_sku_check" CHECK (length("product_sku") <= 40),
    CONSTRAINT "fixturesales_invoice_lines_description_check" CHECK (length("description") <= 200),
    CONSTRAINT "fixturesales_invoice_lines_quantity_check" CHECK ("quantity" >= 0.001),
    CONSTRAINT "fixturesales_invoice_lines_unit_price_check" CHECK ("unit_price" >= 0)
);

CREATE TABLE "fixturesales_invoice_tags" (
    "invoice_id" uuid NOT NULL,
    "tag_id" uuid NOT NULL,
    "remark" text,
    CONSTRAINT "fixturesales_invoice_tags_pkey" PRIMARY KEY ("invoice_id", "tag_id"),
    CONSTRAINT "fixturesales_invoice_tags_remark_check" CHECK (length("remark") <= 100)
);

CREATE TABLE "fixturesales_invoices" (
    "id" uuid NOT NULL,
    "version" bigint DEFAULT 1 NOT NULL,
    "created_at" timestamp with time zone DEFAULT now() NOT NULL,
    "updated_at" timestamp with time zone DEFAULT now() NOT NULL,
    "created_by" uuid,
    "updated_by" uuid,
    "status" text DEFAULT 'draft' NOT NULL,
    "amended_from_id" uuid,
    "discount_rate" numeric(5,2) DEFAULT 0 NOT NULL,
    "tax_rate" numeric(5,2) DEFAULT 11 NOT NULL,
    "shipping_street" text,
    "shipping_city" text,
    "shipping_postal_code" text,
    "number" text NOT NULL,
    "issued_on" date NOT NULL,
    "due_on" date NOT NULL,
    "payment_term" text DEFAULT 'net_30' NOT NULL,
    "branch_id" uuid,
    "taxable" boolean DEFAULT true NOT NULL,
    "ship_to_customer" boolean DEFAULT false NOT NULL,
    "customer_name" text NOT NULL,
    "total" numeric(18,2) NOT NULL,
    "approval_note" text,
    "billing_status" text DEFAULT 'to_bill' NOT NULL,
    "delivery_status" text DEFAULT 'to_deliver' NOT NULL,
    "customer_id" uuid NOT NULL,
    CONSTRAINT "fixturesales_invoices_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "fixturesales_invoices_number_key" UNIQUE ("number"),
    CONSTRAINT "fixturesales_invoices_version_check" CHECK ("version" >= 1),
    CONSTRAINT "fixturesales_invoices_status_check" CHECK ("status" IN ('draft', 'deleted', 'need_approval', 'accepted', 'rejected', 'cancelled', 'done')),
    CONSTRAINT "fixturesales_invoices_discount_rate_check" CHECK ("discount_rate" >= 0.00 AND "discount_rate" <= 100.00),
    CONSTRAINT "fixturesales_invoices_tax_rate_check" CHECK ("tax_rate" >= 0.00 AND "tax_rate" <= 100.00),
    CONSTRAINT "fixturesales_invoices_shipping_street_check" CHECK (length("shipping_street") <= 200),
    CONSTRAINT "fixturesales_invoices_shipping_city_check" CHECK (length("shipping_city") <= 100),
    CONSTRAINT "fixturesales_invoices_shipping_postal_code_check" CHECK (length("shipping_postal_code") <= 10),
    CONSTRAINT "fixturesales_invoices_number_check" CHECK (length("number") <= 32),
    CONSTRAINT "fixturesales_invoices_payment_term_check" CHECK ("payment_term" IN ('cash', 'net_15', 'net_30', 'net_60')),
    CONSTRAINT "fixturesales_invoices_customer_name_check" CHECK (length("customer_name") <= 200),
    CONSTRAINT "fixturesales_invoices_approval_note_check" CHECK (length("approval_note") <= 1000),
    CONSTRAINT "fixturesales_invoices_billing_status_check" CHECK ("billing_status" IN ('to_bill', 'billed')),
    CONSTRAINT "fixturesales_invoices_delivery_status_check" CHECK ("delivery_status" IN ('to_deliver', 'partially_delivered', 'delivered'))
);

CREATE TABLE "fixturesales_notes" (
    "id" uuid NOT NULL,
    "version" bigint DEFAULT 1 NOT NULL,
    "created_at" timestamp with time zone DEFAULT now() NOT NULL,
    "updated_at" timestamp with time zone DEFAULT now() NOT NULL,
    "created_by" uuid,
    "updated_by" uuid,
    "title" text NOT NULL,
    "body" text,
    "subject_type" text NOT NULL,
    "subject_id" uuid NOT NULL,
    CONSTRAINT "fixturesales_notes_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "fixturesales_notes_version_check" CHECK ("version" >= 1),
    CONSTRAINT "fixturesales_notes_title_check" CHECK (length("title") <= 120),
    CONSTRAINT "fixturesales_notes_body_check" CHECK (length("body") <= 4000),
    CONSTRAINT "fixturesales_notes_subject_type_check" CHECK ("subject_type" IN ('fixturesales.invoice', 'fixturesales.product'))
);

CREATE TABLE "fixturesales_products" (
    "id" uuid NOT NULL,
    "version" bigint DEFAULT 1 NOT NULL,
    "created_at" timestamp with time zone DEFAULT now() NOT NULL,
    "updated_at" timestamp with time zone DEFAULT now() NOT NULL,
    "created_by" uuid,
    "updated_by" uuid,
    "sku" text NOT NULL,
    "name" text NOT NULL,
    "unit_price" numeric(18,2) NOT NULL,
    "active" boolean DEFAULT true NOT NULL,
    CONSTRAINT "fixturesales_products_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "fixturesales_products_sku_key" UNIQUE ("sku"),
    CONSTRAINT "fixturesales_products_version_check" CHECK ("version" >= 1),
    CONSTRAINT "fixturesales_products_sku_check" CHECK (length("sku") <= 40),
    CONSTRAINT "fixturesales_products_name_check" CHECK (length("name") <= 200),
    CONSTRAINT "fixturesales_products_unit_price_check" CHECK ("unit_price" >= 0)
);

CREATE TABLE "fixturesales_tags" (
    "id" uuid NOT NULL,
    "version" bigint DEFAULT 1 NOT NULL,
    "created_at" timestamp with time zone DEFAULT now() NOT NULL,
    "updated_at" timestamp with time zone DEFAULT now() NOT NULL,
    "created_by" uuid,
    "updated_by" uuid,
    "name" text NOT NULL,
    CONSTRAINT "fixturesales_tags_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "fixturesales_tags_name_key" UNIQUE ("name"),
    CONSTRAINT "fixturesales_tags_version_check" CHECK ("version" >= 1),
    CONSTRAINT "fixturesales_tags_name_check" CHECK (length("name") <= 60)
);

CREATE INDEX "fixturecore_customers_status_name_idx" ON "fixturecore_customers" ("status", "name");

CREATE INDEX "fixturesales_invoice_lines_line_no_idx" ON "fixturesales_invoice_lines" ("line_no");

CREATE INDEX "fixturesales_invoices_status_issued_on_idx" ON "fixturesales_invoices" ("status", "issued_on");

ALTER TABLE "fixturesales_invoice_lines" ADD CONSTRAINT "fixturesales_invoice_lines_invoice_id_fkey" FOREIGN KEY ("invoice_id") REFERENCES "fixturesales_invoices" ("id") ON DELETE CASCADE;

ALTER TABLE "fixturesales_invoice_lines" ADD CONSTRAINT "fixturesales_invoice_lines_product_id_fkey" FOREIGN KEY ("product_id") REFERENCES "fixturesales_products" ("id") ON DELETE RESTRICT;

ALTER TABLE "fixturesales_invoice_tags" ADD CONSTRAINT "fixturesales_invoice_tags_invoice_id_fkey" FOREIGN KEY ("invoice_id") REFERENCES "fixturesales_invoices" ("id") ON DELETE CASCADE;

ALTER TABLE "fixturesales_invoice_tags" ADD CONSTRAINT "fixturesales_invoice_tags_tag_id_fkey" FOREIGN KEY ("tag_id") REFERENCES "fixturesales_tags" ("id") ON DELETE CASCADE;

CREATE FUNCTION "fixturesales_invoice_lines_invoice_id_lock_fn"() RETURNS trigger LANGUAGE plpgsql AS $erpora$
BEGIN
  IF TG_OP IN ('INSERT', 'UPDATE') THEN
    PERFORM 1 FROM "fixturesales_invoices" AS p WHERE p."id" = NEW."invoice_id" FOR SHARE OF p;
    PERFORM 1 FROM "fixturesales_invoices" AS p WHERE p."id" = NEW."invoice_id" AND p."status" IS DISTINCT FROM 'draft';
    IF FOUND THEN
      RAISE EXCEPTION 'dokumen induk terkunci: perubahan anak ditolak (fixturesales_invoice_lines)' USING ERRCODE = '55000';
    END IF;
  END IF;
  IF TG_OP = 'DELETE' OR (TG_OP = 'UPDATE' AND OLD."invoice_id" IS DISTINCT FROM NEW."invoice_id") THEN
    PERFORM 1 FROM "fixturesales_invoices" AS p WHERE p."id" = OLD."invoice_id" FOR SHARE OF p;
    PERFORM 1 FROM "fixturesales_invoices" AS p WHERE p."id" = OLD."invoice_id" AND p."status" IS DISTINCT FROM 'draft';
    IF FOUND THEN
      RAISE EXCEPTION 'dokumen induk terkunci: perubahan anak ditolak (fixturesales_invoice_lines)' USING ERRCODE = '55000';
    END IF;
  END IF;
  IF TG_OP = 'DELETE' THEN
    RETURN OLD;
  END IF;
  RETURN NEW;
END
$erpora$;

CREATE TRIGGER "fixturesales_invoice_lines_invoice_id_lock_trg" BEFORE DELETE OR INSERT OR UPDATE ON "fixturesales_invoice_lines" FOR EACH ROW EXECUTE FUNCTION "fixturesales_invoice_lines_invoice_id_lock_fn"();

ALTER TABLE "fixturesales_invoice_lines" ENABLE ALWAYS TRIGGER "fixturesales_invoice_lines_invoice_id_lock_trg";

CREATE FUNCTION "fixturesales_invoice_tags_invoice_id_lock_fn"() RETURNS trigger LANGUAGE plpgsql AS $erpora$
BEGIN
  IF TG_OP IN ('INSERT', 'UPDATE') THEN
    PERFORM 1 FROM "fixturesales_invoices" AS p WHERE p."id" = NEW."invoice_id" FOR SHARE OF p;
    PERFORM 1 FROM "fixturesales_invoices" AS p WHERE p."id" = NEW."invoice_id" AND p."status" IS DISTINCT FROM 'draft';
    IF FOUND THEN
      RAISE EXCEPTION 'dokumen induk terkunci: perubahan anak ditolak (fixturesales_invoice_tags)' USING ERRCODE = '55000';
    END IF;
  END IF;
  IF TG_OP = 'DELETE' OR (TG_OP = 'UPDATE' AND OLD."invoice_id" IS DISTINCT FROM NEW."invoice_id") THEN
    PERFORM 1 FROM "fixturesales_invoices" AS p WHERE p."id" = OLD."invoice_id" FOR SHARE OF p;
    PERFORM 1 FROM "fixturesales_invoices" AS p WHERE p."id" = OLD."invoice_id" AND p."status" IS DISTINCT FROM 'draft';
    IF FOUND THEN
      RAISE EXCEPTION 'dokumen induk terkunci: perubahan anak ditolak (fixturesales_invoice_tags)' USING ERRCODE = '55000';
    END IF;
  END IF;
  IF TG_OP = 'DELETE' THEN
    RETURN OLD;
  END IF;
  RETURN NEW;
END
$erpora$;

CREATE TRIGGER "fixturesales_invoice_tags_invoice_id_lock_trg" BEFORE DELETE OR INSERT OR UPDATE ON "fixturesales_invoice_tags" FOR EACH ROW EXECUTE FUNCTION "fixturesales_invoice_tags_invoice_id_lock_fn"();

ALTER TABLE "fixturesales_invoice_tags" ENABLE ALWAYS TRIGGER "fixturesales_invoice_tags_invoice_id_lock_trg";

CREATE FUNCTION "fixturesales_invoices_lock_fn"() RETURNS trigger LANGUAGE plpgsql AS $erpora$
BEGIN
  IF TG_OP = 'DELETE' THEN
    IF OLD."status" IS DISTINCT FROM 'draft' THEN
      RAISE EXCEPTION 'dokumen terkunci: perubahan ditolak (fixturesales_invoices)' USING ERRCODE = '55000';
    END IF;
    RETURN OLD;
  END IF;
  IF NOT (OLD."status" IS DISTINCT FROM 'draft') THEN
    RETURN NEW;
  END IF;
  -- transisi status lifecycle yang sah
  IF (to_jsonb(NEW) - ARRAY['status', 'updated_at', 'updated_by', 'version']) = (to_jsonb(OLD) - ARRAY['status', 'updated_at', 'updated_by', 'version']) AND (OLD."status", NEW."status") IN (('draft', 'deleted'), ('draft', 'accepted'), ('draft', 'need_approval'), ('need_approval', 'accepted'), ('need_approval', 'rejected'), ('accepted', 'cancelled'), ('accepted', 'done')) THEN
    RETURN NEW;
  END IF;
  -- transisi sumbu status yang sah (boleh mundur)
  IF (to_jsonb(NEW) - ARRAY['billing_status', 'delivery_status', 'updated_at', 'updated_by', 'version']) = (to_jsonb(OLD) - ARRAY['billing_status', 'delivery_status', 'updated_at', 'updated_by', 'version']) AND (OLD."billing_status" IS NOT DISTINCT FROM NEW."billing_status" OR (OLD."billing_status", NEW."billing_status") IN (('to_bill', 'billed'))) AND (OLD."delivery_status" IS NOT DISTINCT FROM NEW."delivery_status" OR (OLD."delivery_status", NEW."delivery_status") IN (('to_deliver', 'partially_delivered'), ('to_deliver', 'delivered'), ('partially_delivered', 'delivered'), ('partially_delivered', 'to_deliver'), ('delivered', 'partially_delivered'), ('delivered', 'to_deliver'))) AND (OLD."billing_status" IS DISTINCT FROM NEW."billing_status" OR OLD."delivery_status" IS DISTINCT FROM NEW."delivery_status") THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'dokumen terkunci: perubahan ditolak (fixturesales_invoices)' USING ERRCODE = '55000';
END
$erpora$;

CREATE TRIGGER "fixturesales_invoices_lock_trg" BEFORE DELETE OR UPDATE ON "fixturesales_invoices" FOR EACH ROW EXECUTE FUNCTION "fixturesales_invoices_lock_fn"();

ALTER TABLE "fixturesales_invoices" ENABLE ALWAYS TRIGGER "fixturesales_invoices_lock_trg";

