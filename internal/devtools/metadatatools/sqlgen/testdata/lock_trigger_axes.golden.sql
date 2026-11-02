CREATE FUNCTION "m_docs_lock_fn"() RETURNS trigger LANGUAGE plpgsql AS $erpora$
BEGIN
  IF TG_OP = 'DELETE' THEN
    IF OLD."status" IS DISTINCT FROM 'draft' THEN
      RAISE EXCEPTION 'dokumen terkunci: perubahan ditolak (m_docs)' USING ERRCODE = '55000';
    END IF;
    RETURN OLD;
  END IF;
  IF NOT (OLD."status" IS DISTINCT FROM 'draft') THEN
    RETURN NEW;
  END IF;
  -- transisi status lifecycle yang sah
  IF (to_jsonb(NEW) - ARRAY['status', 'updated_at', 'updated_by', 'version']) = (to_jsonb(OLD) - ARRAY['status', 'updated_at', 'updated_by', 'version']) AND (OLD."status", NEW."status") IN (('draft', 'submitted'), ('submitted', 'cancelled')) THEN
    RETURN NEW;
  END IF;
  -- transisi sumbu status yang sah (boleh mundur)
  IF (to_jsonb(NEW) - ARRAY['billing_status', 'delivery_status', 'updated_at', 'updated_by', 'version']) = (to_jsonb(OLD) - ARRAY['billing_status', 'delivery_status', 'updated_at', 'updated_by', 'version']) AND (OLD."billing_status" IS NOT DISTINCT FROM NEW."billing_status" OR (OLD."billing_status", NEW."billing_status") IN (('to_bill', 'billed'))) AND (OLD."delivery_status" IS NOT DISTINCT FROM NEW."delivery_status" OR (OLD."delivery_status", NEW."delivery_status") IN (('to_deliver', 'partially_delivered'), ('partially_delivered', 'to_deliver'))) AND (OLD."billing_status" IS DISTINCT FROM NEW."billing_status" OR OLD."delivery_status" IS DISTINCT FROM NEW."delivery_status") THEN
    RETURN NEW;
  END IF;
  -- pengisian kolom tujuan migrateFrom
  IF (to_jsonb(NEW) - ARRAY['phone_e164']) = (to_jsonb(OLD) - ARRAY['phone_e164']) AND (NEW."phone_e164" IS NOT DISTINCT FROM OLD."phone_e164" OR (OLD."phone_e164" IS NULL AND NEW."phone_e164" IS NOT DISTINCT FROM OLD."phone")) AND (NEW."phone_e164" IS DISTINCT FROM OLD."phone_e164") THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'dokumen terkunci: perubahan ditolak (m_docs)' USING ERRCODE = '55000';
END
$erpora$;

CREATE TRIGGER "m_docs_lock_trg" BEFORE DELETE OR UPDATE ON "m_docs" FOR EACH ROW EXECUTE FUNCTION "m_docs_lock_fn"();

ALTER TABLE "m_docs" ENABLE ALWAYS TRIGGER "m_docs_lock_trg";

