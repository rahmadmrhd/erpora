CREATE FUNCTION "m_docs_truncate_lock_fn"() RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog, pg_temp AS $erpora$
DECLARE
  is_locked boolean;
BEGIN
  IF current_setting('transaction_isolation') <> 'read committed' THEN
    RAISE EXCEPTION 'TRUNCATE tabel dokumen ditolak pada transaction_isolation selain read committed (m_docs): jalankan pada READ COMMITTED' USING ERRCODE = '55000';
  END IF;
  EXECUTE format('SELECT EXISTS (SELECT 1 FROM %I.%I AS t WHERE t."status" IS DISTINCT FROM ''draft'')', TG_TABLE_SCHEMA, TG_TABLE_NAME) INTO is_locked;
  IF is_locked THEN
    RAISE EXCEPTION 'dokumen terkunci: TRUNCATE ditolak (m_docs)' USING ERRCODE = '55000';
  END IF;
  RETURN NULL;
END
$erpora$;

CREATE TRIGGER "m_docs_truncate_lock_trg" BEFORE TRUNCATE ON "m_docs" FOR EACH STATEMENT EXECUTE FUNCTION "m_docs_truncate_lock_fn"();

ALTER TABLE "m_docs" ENABLE ALWAYS TRIGGER "m_docs_truncate_lock_trg";

