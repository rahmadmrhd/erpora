# Kebijakan Keamanan ERPora

## Melaporkan kerentanan

Jangan membuka issue publik untuk kerentanan. Laporkan secara privat lewat salah satu cara berikut:

- **GitHub private vulnerability reporting** (tab *Security* > *Report a vulnerability* / security advisories) pada repositori ini.
- Email: `rahmadm.rhd@gmail.com`.

Sertakan deskripsi, langkah reproduksi, versi/commit yang terdampak, dan dampak yang Anda perkirakan.
Kami akan mengonfirmasi penerimaan laporan dan memberi kabar perkembangan perbaikan.

## Versi yang didukung

| Versi | Didukung |
| --- | --- |
| `main` | Ya |
| Rilis lain | Belum ada (belum ada rilis bertag) |

## Target waktu perbaikan

| Tingkat keparahan | Target perbaikan |
| --- | --- |
| Critical | 7 hari |
| High | 30 hari |
| Medium | 90 hari |
| Low | Rilis berikutnya |

Tingkat keparahan dinilai dengan CVSS dan konteks penggunaan. Target dihitung sejak laporan dikonfirmasi valid.

## Pemeriksaan otomatis

CI menjalankan `gosec` (lewat golangci-lint), `govulncheck`, dan pemindaian rahasia (gitleaks) pada setiap pull request.
