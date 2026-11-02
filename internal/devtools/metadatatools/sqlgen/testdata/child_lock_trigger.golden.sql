CREATE FUNCTION "m_doc_lines_doc_id_lock_fn"() RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog, pg_temp AS $erpora$
DECLARE
  parent_locked boolean;
BEGIN
  IF TG_OP IN ('INSERT', 'UPDATE') THEN
    EXECUTE format('SELECT 1 FROM %I.%I AS p WHERE p."id" = $1 FOR SHARE OF p', TG_TABLE_SCHEMA, 'm_docs') USING NEW."doc_id";
    EXECUTE format('SELECT EXISTS (SELECT 1 FROM %I.%I AS p WHERE p."id" = $1 AND p."status" IS DISTINCT FROM ''draft'')', TG_TABLE_SCHEMA, 'm_docs') INTO parent_locked USING NEW."doc_id";
    IF parent_locked THEN
      RAISE EXCEPTION 'dokumen induk terkunci: perubahan anak ditolak (m_doc_lines)' USING ERRCODE = '55000';
    END IF;
  END IF;
  IF TG_OP = 'DELETE' OR (TG_OP = 'UPDATE' AND OLD."doc_id" IS DISTINCT FROM NEW."doc_id") THEN
    EXECUTE format('SELECT 1 FROM %I.%I AS p WHERE p."id" = $1 FOR SHARE OF p', TG_TABLE_SCHEMA, 'm_docs') USING OLD."doc_id";
    EXECUTE format('SELECT EXISTS (SELECT 1 FROM %I.%I AS p WHERE p."id" = $1 AND p."status" IS DISTINCT FROM ''draft'')', TG_TABLE_SCHEMA, 'm_docs') INTO parent_locked USING OLD."doc_id";
    IF parent_locked THEN
      RAISE EXCEPTION 'dokumen induk terkunci: perubahan anak ditolak (m_doc_lines)' USING ERRCODE = '55000';
    END IF;
  END IF;
  IF TG_OP = 'DELETE' THEN
    RETURN OLD;
  END IF;
  RETURN NEW;
END
$erpora$;

CREATE TRIGGER "m_doc_lines_doc_id_lock_trg" BEFORE DELETE OR INSERT OR UPDATE ON "m_doc_lines" FOR EACH ROW EXECUTE FUNCTION "m_doc_lines_doc_id_lock_fn"();

ALTER TABLE "m_doc_lines" ENABLE ALWAYS TRIGGER "m_doc_lines_doc_id_lock_trg";

