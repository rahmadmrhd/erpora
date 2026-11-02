DO $erpora$
BEGIN
  IF EXISTS (SELECT 1 FROM "m_things" WHERE "phone" IS NOT NULL AND "phone_e164" IS NULL) THEN
    RAISE EXCEPTION 'penghapusan kolom ditolak: masih ada baris dengan kolom lama terisi dan kolom pengganti kosong (m_things)';
  END IF;
END
$erpora$;
