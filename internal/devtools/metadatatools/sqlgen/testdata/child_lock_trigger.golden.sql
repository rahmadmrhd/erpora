CREATE FUNCTION "m_doc_lines_doc_id_lock_fn"() RETURNS trigger LANGUAGE plpgsql AS $erpora$
BEGIN
  IF TG_OP IN ('INSERT', 'UPDATE') THEN
    PERFORM 1 FROM "m_docs" AS p WHERE p."id" = NEW."doc_id" FOR SHARE OF p;
    PERFORM 1 FROM "m_docs" AS p WHERE p."id" = NEW."doc_id" AND p."status" IS DISTINCT FROM 'draft';
    IF FOUND THEN
      RAISE EXCEPTION 'dokumen induk terkunci: perubahan anak ditolak (m_doc_lines)' USING ERRCODE = '55000';
    END IF;
  END IF;
  IF TG_OP = 'DELETE' OR (TG_OP = 'UPDATE' AND OLD."doc_id" IS DISTINCT FROM NEW."doc_id") THEN
    PERFORM 1 FROM "m_docs" AS p WHERE p."id" = OLD."doc_id" FOR SHARE OF p;
    PERFORM 1 FROM "m_docs" AS p WHERE p."id" = OLD."doc_id" AND p."status" IS DISTINCT FROM 'draft';
    IF FOUND THEN
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

