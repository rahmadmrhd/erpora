DO $erpora$
DECLARE
  affected bigint;
BEGIN
  LOOP
    UPDATE "m_things" SET "phone_e164" = "phone" WHERE "id" IN (SELECT "id" FROM "m_things" WHERE "phone_e164" IS NULL AND "phone" IS NOT NULL ORDER BY "id" LIMIT 500);
    GET DIAGNOSTICS affected = ROW_COUNT;
    EXIT WHEN affected = 0;
    COMMIT;
  END LOOP;
END
$erpora$;
