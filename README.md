# BAAK API

> [!NOTE]  
> **v2.0.0** - Now supports FlareSolverr for bypassing Cloudflare protection! See configuration below.

> [!IMPORTANT]  
> The BAAK website uses Cloudflare protection. For full functionality, you'll need to set up [FlareSolverr](https://github.com/FlareSolverr/FlareSolverr).

An unofficial API for BAAK.

## Disclaimer

Perlu diketahui bahwa proyek ini tidak berafiliasi dengan Universitas Gunadarma maupun BAAK. Proyek ini dibuat murni untuk tujuan pembelajaran dan pengembangan keterampilan. Mohon untuk tidak menggunakan API ini untuk hal-hal yang tidak semestinya. Developer tidak bertanggung jawab atas penyalahgunaan yang mungkin terjadi dari penggunaan API ini.

## Fitur

- Pencarian Jadwal Kuliah
- Kalender Akademik
- Informasi Kelas Baru
- Jadwal UTS
- Informasi Mahasiswa Baru
- Rate limiting (per-IP)
- Dukungan CORS
- Monitoring kesehatan (dengan status komponen)
- Format error yang terstandarisasi
- **NEW**: FlareSolverr support untuk Cloudflare bypass
- **NEW**: Circuit breaker pattern untuk resiliensi
- **NEW**: In-memory caching untuk mengurangi beban

## Endpoint API

### Health Check

```
GET /health
```

Mengembalikan status kesehatan API dengan detail komponen:
- Status FlareSolverr
- Status circuit breaker
- Statistik cache

### Jadwal Kuliah

```
GET /jadwal/{kelas}
```

Mendapatkan informasi jadwal untuk kelas tertentu.

Parameter:

- `kelas` (path parameter): Kode kelas (minimal 3 karakter)

### Kalender Akademik

```
GET /kalender
```

Mendapatkan informasi kalender akademik.

### Informasi Kelas Baru

```
GET /kelasbaru/{kelas}
```

Mendapatkan informasi tentang kelas baru.

Parameter:

- `kelas` (path parameter): Kode kelas

### Jadwal UTS

```
GET /uts/{kelas}
```

Mendapatkan jadwal UTS (Ujian Tengah Semester) untuk kelas tertentu.

Parameter:

- `kelas` (path parameter): Kode kelas

### Informasi Mahasiswa Baru

```
GET /mahasiswabaru/{npm}
```

Mendapatkan informasi untuk mahasiswa baru.

Parameter:

- `npm` (path parameter): Nomor Pokok Mahasiswa

## Format Response

Semua response mengikuti format ini:

```json
{
  "success": true,
  "data": {
    // Data response di sini
  }
}
```

Response error:

```json
{
  "success": false,
  "error": "Pesan error di sini",
  "code": "ERROR_CODE"
}
```

Error codes:
- `VALIDATION_ERROR` - Input tidak valid
- `NOT_FOUND` - Resource tidak ditemukan
- `CLOUDFLARE_BLOCKED` - Terblokir oleh Cloudflare
- `CIRCUIT_OPEN` - Circuit breaker terbuka
- `FLARESOLVERR_ERROR` - Error saat menggunakan FlareSolverr
- `RATE_LIMITED` - Terlalu banyak request
- `UPSTREAM_ERROR` - Error dari server BAAK
- `SESSION_ERROR` - Gagal membuat session

## Rate Limiting

API ini menggunakan per-IP rate limiting untuk mencegah penyalahgunaan. Default: 5 request per detik dengan burst 10.

## Konfigurasi

API bisa dikonfigurasi menggunakan environment variables:

| Variable | Default | Deskripsi |
|----------|---------|-----------|
| `PORT` | `:8080` | Port server |
| `BASE_URL` | `https://baak.gunadarma.ac.id` | URL dasar website BAAK |
| `RATE_LIMIT_PER_MIN` | `60` | Batas rate per menit (deprecated, now per-IP) |
| `ALLOWED_ORIGINS` | `*` | Daftar origin CORS yang diizinkan |
| `FLARESOLVERR_URL` | - | URL FlareSolverr (e.g., `http://localhost:8191`) |
| `CACHE_TTL_JADWAL` | `300` | TTL cache jadwal dalam detik |
| `CACHE_TTL_KALENDER` | `3600` | TTL cache kalender dalam detik |
| `CACHE_ENABLED` | `true` | Enable/disable caching |

## FlareSolverr Setup

FlareSolverr diperlukan untuk melewati proteksi Cloudflare. Jalankan dengan Docker:

```bash
docker run -d \
  --name flaresolverr \
  -p 8191:8191 \
  ghcr.io/flaresolverr/flaresolverr:latest
```

Kemudian set environment variable:

```bash
export FLARESOLVERR_URL=http://localhost:8191
```

## Development

### Prasyarat

- Go 1.22 atau lebih tinggi
- Git
- Docker (untuk FlareSolverr)

### Setup

1. Clone repository:

```bash
git clone https://github.com/yourusername/baak-api.git
cd baak-api
```

2. Install dependencies:

```bash
go mod download
```

3. (Optional) Jalankan FlareSolverr:

```bash
docker run -d -p 8191:8191 ghcr.io/flaresolverr/flaresolverr:latest
```

4. Jalankan server:

```bash
FLARESOLVERR_URL=http://localhost:8191 go run main.go
```

## Architecture

```
├── api/           # Vercel handler & routing
├── config/        # Configuration management
├── handlers/      # HTTP request handlers
├── middleware/    # HTTP middleware (CORS, rate limiting, etc.)
├── models/        # Data structures
└── utils/         # Utilities (caching, circuit breaker, FlareSolverr, etc.)
```

## To-Do

- [x] Jadwal
- [x] Kalender Akademik
- [x] Mahasiswa Baru
- [x] Mahasiswa Kelas 2 Baru
- [x] UTS
- [x] FlareSolverr Integration
- [x] Circuit Breaker
- [x] Caching Layer
- [ ] UU
- [ ] UAS

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

