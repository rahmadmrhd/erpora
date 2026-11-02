CREATE FUNCTION "s"."m_docs_lock_fn"() RETURNS trigger LANGUAGE plpgsql AS $erpora2$
BEGIN
  IF TG_OP = 'DELETE' THEN
    IF OLD."status" IS DISTINCT FROM 'x$erpora$ ; DROP TABLE m_docs; -- $erpora1$' THEN
      RAISE EXCEPTION 'dokumen terkunci: perubahan ditolak (m_docs)' USING ERRCODE = '55000';
    END IF;
    RETURN OLD;
  END IF;
  IF NOT (OLD."status" IS DISTINCT FROM 'x$erpora$ ; DROP TABLE m_docs; -- $erpora1$') THEN
    RETURN NEW;
  END IF;
  -- transisi status lifecycle yang sah
  IF (to_jsonb(NEW) - ARRAY['status', 'updated_at', 'updated_by', 'version']) = (to_jsonb(OLD) - ARRAY['status', 'updated_at', 'updated_by', 'version']) AND (OLD."status", NEW."status") IN (('x$erpora$ ; DROP TABLE m_docs; -- $erpora1$', 'y''z')) THEN
    RETURN NEW;
  END IF;
  -- pengisian kolom tujuan migrateFrom
  IF (to_jsonb(NEW) - ARRAY['phone_e164']) = (to_jsonb(OLD) - ARRAY['phone_e164']) AND (NEW."phone_e164" IS NOT DISTINCT FROM OLD."phone_e164" OR (OLD."phone_e164" IS NULL AND NEW."phone_e164" IS NOT DISTINCT FROM OLD."phone")) AND (NEW."phone_e164" IS DISTINCT FROM OLD."phone_e164") THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'dokumen terkunci: perubahan ditolak (m_docs)' USING ERRCODE = '55000';
END
$erpora2$;

CREATE TRIGGER "m_docs_lock_trg" BEFORE DELETE OR UPDATE ON "s"."m_docs" FOR EACH ROW EXECUTE FUNCTION "s"."m_docs_lock_fn"();

ALTER TABLE "s"."m_docs" ENABLE ALWAYS TRIGGER "m_docs_lock_trg";

