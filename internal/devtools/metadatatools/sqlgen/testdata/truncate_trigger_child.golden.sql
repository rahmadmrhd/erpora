CREATE FUNCTION "m_doc_lines_doc_id_truncate_lock_fn"() RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog, pg_temp AS $erpora$
DECLARE
  is_locked boolean;
BEGIN
  IF current_setting('transaction_isolation') <> 'read committed' THEN
    RAISE EXCEPTION 'TRUNCATE tabel dokumen ditolak pada transaction_isolation selain read committed (m_doc_lines): jalankan pada READ COMMITTED' USING ERRCODE = '55000';
  END IF;
  EXECUTE format('SELECT EXISTS (SELECT 1 FROM %I.%I AS c JOIN %I.%I AS p ON p."id" = c."doc_id" WHERE p."status" IS DISTINCT FROM ''draft'')', TG_TABLE_SCHEMA, TG_TABLE_NAME, TG_TABLE_SCHEMA, 'm_docs') INTO is_locked;
  IF is_locked THEN
    RAISE EXCEPTION 'dokumen induk terkunci: TRUNCATE anak ditolak (m_doc_lines)' USING ERRCODE = '55000';
  END IF;
  RETURN NULL;
END
$erpora$;

CREATE TRIGGER "m_doc_lines_doc_id_truncate_lock_trg" BEFORE TRUNCATE ON "m_doc_lines" FOR EACH STATEMENT EXECUTE FUNCTION "m_doc_lines_doc_id_truncate_lock_fn"();

ALTER TABLE "m_doc_lines" ENABLE ALWAYS TRIGGER "m_doc_lines_doc_id_truncate_lock_trg";

