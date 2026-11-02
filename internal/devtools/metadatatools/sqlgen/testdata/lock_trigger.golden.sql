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
  -- pengisian kolom tujuan migrateFrom
  IF (to_jsonb(NEW) - ARRAY['phone_e164']) = (to_jsonb(OLD) - ARRAY['phone_e164']) AND ((to_jsonb(NEW) -> 'phone_e164') = (to_jsonb(OLD) -> 'phone_e164') OR ((to_jsonb(OLD) -> 'phone_e164') = 'null'::jsonb AND (to_jsonb(NEW) -> 'phone_e164') = COALESCE(to_jsonb(OLD) -> 'phone', 'null'::jsonb))) AND ((to_jsonb(NEW) -> 'phone_e164') IS DISTINCT FROM (to_jsonb(OLD) -> 'phone_e164')) THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'dokumen terkunci: perubahan ditolak (m_docs)' USING ERRCODE = '55000';
END
$erpora$;

CREATE TRIGGER "m_docs_lock_trg" BEFORE DELETE OR UPDATE ON "m_docs" FOR EACH ROW EXECUTE FUNCTION "m_docs_lock_fn"();

ALTER TABLE "m_docs" ENABLE ALWAYS TRIGGER "m_docs_lock_trg";

