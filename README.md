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
- Endpoint publik read-only untuk katalog akademik, ujian, berita, dokumen, dan prosedur layanan

## Endpoint API

### Health Check

```
GET /health
```

Mengembalikan status kesehatan API dengan detail komponen:
- Status FlareSolverr
- Status circuit breaker
- Statistik cache

`/health` adalah readiness probe dan dapat mengembalikan HTTP 503 saat
FlareSolverr tidak sehat atau circuit breaker sedang menolak request. Setelah
masa tunggu circuit berakhir, readiness mengizinkan trafik kembali agar probe
pemulihan dapat berjalan. Field `circuit_breaker.accepting_requests` menunjukkan
kesiapan menerima request. Gunakan `/live` sebagai liveness probe yang tidak
bergantung pada BAAK atau FlareSolverr, dan `/ready` sebagai alias readiness.

### Jadwal Kuliah

```
GET /jadwal/{kelas}
GET /jadwal?q={kelas_atau_dosen}
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

### Form pencarian resmi BAAK

API juga mempertahankan kontrak GET yang tampil pada form publik BAAK:

```
GET /cariKelasBaru?tipeKelasBaru=Kelas|NPM|Nama&teks=...
GET /cariMhsBaru?tipeMhsBaru=Kelas|Nama&teks=...
```

Keduanya hanya membaca hasil pencarian dan mengembalikan envelope JSON yang sama
dengan endpoint `/kelasbaru/{kelas}` dan `/mahasiswabaru/{kelas_atau_nama}`.

### Informasi Mahasiswa Baru

```
GET /mahasiswabaru/{kelas_atau_nama}
```

Mendapatkan informasi untuk mahasiswa baru.

Parameter:

- `kelas_atau_nama` (path parameter): Kode kelas atau nama mahasiswa

### Katalog Publik BAAK

```text
GET /catalog
GET /public/{section}
```

`/catalog` mengembalikan daftar section publik yang tersedia. Semua section berikut
bersifat read-only dan mengembalikan JSON terstandardisasi dengan `title`, `text`,
`links`, `tables`, `records`, `options`, serta `pagination` jika tersedia:

- `/ujian-utama?jurusan=...` - Jadwal Ujian Utama per jurusan
- `/uas/{kelas}` - Jadwal UAS per kelas
- `/mata-kuliah` - Daftar mata kuliah dan dokumen katalog
- `/dosen-wali/{tingkat}` - Daftar dosen wali kelas
- `/koordinator` - Koordinator mata kuliah, termasuk pagination
- `/pembimbing-pi` - Dosen pembimbing dan mahasiswa PI
- `/panduan-kuliah` dan `/jadwal-ujian` - Informasi panduan perkuliahan/ujian
- `/ujian-bentrok` dan `/frs` - Prosedur publik dan dokumen terkait
- `/berita` atau `/berita/{id}` - Daftar/detail berita BAAK
- `/buku-pedoman` - Daftar buku pedoman dan tautan dokumen
- `/dokumen/{kategori}/{id}/teks` - Teks, tabel, halaman, dan metadata PDF; `id` berasal dari `id` pada link katalog
- `/layanan/{slug}` - `daftar-ulang`, `cuti`, `nonaktif`, `pengecekan-nilai`, `pindah-lokasi`, `pindah-jurusan`
- `/situs` dan `/loket` - Tautan situs resmi dan jam pelayanan loket

Endpoint katalog hanya membaca halaman sumber. Ia tidak melakukan perubahan data,
pengisian KRS, daftar ulang, pengajuan cuti, atau pengajuan ujian bentrok. UAS
mengikuti formulir BAAK dengan session dan CSRF token, sedangkan Ujian Utama memakai
parameter `jurusan` yang dipilih BAAK.

### Audit cakupan BAAK (11 September 2026)

Audit browser lokal terhadap navigasi dan enam halaman arsip berita menemukan
halaman publik berikut: beranda/ringkasan, jadwal kuliah, Ujian Utama, UAS,
kalender, daftar mata kuliah, dosen wali, koordinator, pembimbing PI, panduan
jadwal kuliah, jadwal ujian, ujian bentrok, FRS, enam prosedur administrasi,
berita (36 detail), buku pedoman, arsip kalender, situs resmi, loket, serta form
pencarian kelas baru dan mahasiswa baru. Semuanya memiliki pemetaan read-only di
API dan diuji dengan fixture lokal atau parser yang sesuai.

Audit juga menemukan aset publik yang sengaja belum masuk endpoint PDF teks:
lampiran PDF pada detail berita, formulir administrasi (`.doc` dan PDF), serta
tautan PDF umum seperti PPSPPT. Endpoint PDF saat ini hanya menerima empat
katalog resmi (`mata-kuliah`, `buku-pedoman`, `frs`, `kalender`) agar resolusi ID
tetap terikat katalog dan tidak berubah menjadi downloader URL bebas. Tautan
tersebut tetap muncul sebagai link publik bila berada di halaman yang dipetakan;
ekstraksi teksnya memerlukan kategori katalog tambahan yang belum ditetapkan.

`/uts/{kelas}` tetap dipertahankan sebagai kompatibilitas lama, tetapi halaman
`/jadwal/cariUts` BAAK saat audit mengembalikan Server Error sehingga API
mengembalikan error upstream, bukan data kosong palsu. Situs eksternal seperti
RPS, sidang, situs jurusan, ujian, formulir, dan live chat hanya diekspos sebagai
tautan; API tidak mem-proxy atau meniru aksinya.

Untuk section tabel yang mendukung pencarian, gunakan `q` atau parameter asli BAAK
(`search_wali`, `search_koor`, `search_pi`) dan `page` 1 sampai 10000. Satu request
mengambil satu halaman sumber, sehingga seluruh daftar PI tetap dapat diakses. Contoh:
`/koordinator?q=algoritma&page=2`. UAS menerima `/uas/1IA01` atau `/uas?q=1IA01`.

Ekstraksi PDF dibatasi 16 MiB input, 100 halaman, 1.000.000 karakter gabungan teks
dan sel tabel, serta 4 MiB output JSON worker. Batas waktu unduhan 15 detik dan
subprocess 20 detik, dengan maksimal dua ekstraksi bersamaan per proses API.
Worker Linux juga membatasi ruang alamat 512 MiB dan waktu CPU 15 detik; batas
memori ini tidak tersedia di Windows. Batas request API keseluruhan tetap 50 detik.
Dokumen hanya boleh berasal dari URL resmi BAAK yang muncul di katalog; endpoint
menolak URL eksternal, redirect keluar, HTML, dan dokumen rusak.

Respons halaman publik memiliki metadata `fetched_at`, `expires_at`, dan
`content_hash` di dalam `data.metadata`. Berita memakai TTL 5 menit, katalog dokumen
1 jam, hasil ekstraksi PDF 24 jam, dan section `PublicPage` lainnya 10 menit.
Envelope dan endpoint lama dipertahankan; respons legacy seperti `/jadwal` dan
`/kalender` tetap memakai format dan konfigurasi TTL sebelumnya.
Hash halaman dihitung dari data hasil parsing tanpa metadata waktu; hash PDF dari
byte PDF sumber. ID dokumen adalah SHA-256 URL, sehingga berbeda dari hash isi PDF.
Detail berita menyertakan `article` dengan ID, judul, tanggal ISO (kosong bila
tanggal sumber tidak dikenali), penulis, isi, dan
lampiran. PDF berbasis scan akan mengembalikan warning bila teks tidak dapat
diekstrak; OCR tidak dijalankan otomatis.

## Format Response

Untuk development ekstraksi PDF, install `pdfplumber==0.11.9` pada Python yang
ditunjuk `PDF_PYTHON`. Dockerfile memasang versi yang sama. Binary Go meng-embed
script worker, tetapi tetap memerlukan Python dan pdfplumber; deployment yang
tidak menyediakannya mengembalikan HTTP 503 `PDF_UNAVAILABLE`. Contoh alur:

1. Ambil `/dokumen/buku-pedoman`, pilih `data.links[].id` untuk tautan PDF.
2. Panggil `/dokumen/buku-pedoman/{id}/teks`.
3. Baca `data.pages[].text` dan `data.pages[].tables`; periksa `data.warnings`.

Metadata menunjukkan waktu pengambilan sumber, bukan tanggal perubahan resmi BAAK.
`expires_at` adalah batas freshness berdasarkan TTL, bukan jaminan sumber belum
berubah. Saat cache dinonaktifkan, metadata tetap diberikan pada setiap fetch.
Cache diperbarui saat TTL habis dan request berikutnya masuk; tidak ada polling
otomatis atau endpoint publik penghapusan cache. PDF yang diblokir Cloudflare tetap
menghasilkan error sumber karena FlareSolverr tidak menyediakan unduhan PDF biner.
Katalog tetap diperiksa sebelum hasil ekstraksi diberikan, termasuk saat PDF
masih ada di cache. Hanya path `/file/*.pdf` (termasuk subdirektori) dan
`/downloadAkademik/{id_numerik}` pada origin `BASE_URL` yang boleh diunduh; maksimal
dua redirect yang tetap memenuhi aturan tersebut. URL bebas, query unduhan,
path traversal, dan origin lain ditolak.

Hasil PDF berupa teks dan tabel umum, belum parser akademik untuk kode mata kuliah,
SKS, atau semester. Struktur tabel bergantung tata letak PDF dan perlu diperiksa
oleh client. PDF scan mungkin tidak memiliki teks, OCR belum tersedia, dan PDF
yang memerlukan password tidak didukung. Error unduhan/ekstraksi tidak dicache.

### OCR opsional untuk PDF scan

[Baidu Unlimited-OCR](https://github.com/baidu/Unlimited-OCR) secara teknis cocok
untuk PDF scan: modelnya mendukung multi-halaman dan keluaran markdown. Namun ini
bukan dependency ringan seperti `pdfplumber`: model BF16 sekitar 3B parameter,
recipe resmi mencantumkan minimum 8 GB VRAM, image vLLM khusus, `trust_remote_code`,
custom logits processor, prompt `<image>`, dan konversi PDF ke gambar dengan
PyMuPDF. Karena itu OCR sebaiknya dijalankan sebagai worker GPU opsional yang
terpisah, dengan antrean, timeout, batas halaman, dan fallback ke hasil teks
`pdfplumber`; image API utama tetap tidak mengklaim OCR tersedia. Deployment
sekarang belum mengaktifkan worker tersebut karena image produksi tidak membawa
model GPU dan Docker worker lokal belum memiliki runtime NVIDIA yang terverifikasi.

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
- `FLARESOLVERR_BUSY` - Kapasitas request FlareSolverr sedang penuh
- `RATE_LIMITED` - Terlalu banyak request
- `UPSTREAM_ERROR` - Error dari server BAAK
- `SESSION_ERROR` - Gagal membuat session
- `UPSTREAM_TIMEOUT` - Pengambilan data melewati batas waktu request
- `REQUEST_CANCELED` - Request dibatalkan sebelum pengambilan data selesai
- `CORS_FORBIDDEN` - Origin preflight tidak diizinkan
- `CORS_METHOD_FORBIDDEN` - Method preflight tidak diizinkan
- `CORS_HEADERS_FORBIDDEN` - Header preflight tidak diizinkan

## Rate Limiting

API memakai token bucket per IP. `RATE_LIMIT_PER_MIN` mengatur laju pengisian token,
default 60 per menit (1 per detik). `RATE_LIMIT_BURST` mengatur jumlah request yang
bisa diterima sekaligus, default 10. Request saat token habis mendapat HTTP 429
dengan kode `RATE_LIMITED`. Preflight `OPTIONS` tidak menghabiskan token.

IP diambil dari koneksi langsung (`RemoteAddr`). `X-Forwarded-For` dan `X-Real-IP`
diabaikan agar client tidak bisa mengganti identitas lewat header. Jika API dipasang
di belakang reverse proxy, isi `TRUSTED_PROXIES` dengan alamat atau CIDR proxy yang
benar-benar dikelola sendiri. Hanya header dari peer yang cocok dengan daftar itu yang
dipakai untuk memisahkan client; nilai lain tetap dianggap berasal dari peer socket.

Nilai rate dan burst harus bilangan bulat positif; nilai tidak valid memakai
default. Sebelumnya rate efektif di-hardcode 5 per detik; sekarang default mengikuti
konfigurasi 60 per menit. Untuk mempertahankan laju lama, set `RATE_LIMIT_PER_MIN=300`.

## Konfigurasi

API bisa dikonfigurasi menggunakan environment variables:

| Variable | Default | Deskripsi |
|----------|---------|-----------|
| `PORT` | `:8080` | Port server |
| `BASE_URL` | `https://baak.gunadarma.ac.id` | URL dasar website BAAK |
| `RATE_LIMIT_PER_MIN` | `60` | Laju pengisian token per menit per IP |
| `RATE_LIMIT_BURST` | `10` | Kapasitas burst per IP |
| `ALLOWED_ORIGINS` | `*` | Origin CORS yang diizinkan, dipisahkan koma |
| `FLARESOLVERR_URL` | - | URL FlareSolverr (e.g., `http://localhost:8191`) |
| `FLARESOLVERR_MAX_CONCURRENT` | `2` | Jumlah request FlareSolverr aktif per proses |
| `PDF_PYTHON` | `python3` | Executable Python untuk ekstraksi PDF (`pdfplumber` diperlukan) |
| `CACHE_TTL_JADWAL` | `300` | TTL cache jadwal dalam detik |
| `CACHE_TTL_KALENDER` | `3600` | TTL cache kalender dalam detik |
| `CACHE_ENABLED` | `true` | Enable/disable caching |
| `CACHE_MAX_ENTRIES` | `1024` | Batas jumlah entry cache dalam memori |
| `HTTP_PROXIES` | - | URL proxy HTTP/HTTPS dipisahkan koma |
| `TRUSTED_PROXIES` | - | Alamat/CIDR reverse proxy yang boleh mengirim `X-Forwarded-For` atau `X-Real-IP` |

### CORS Lokal

`ALLOWED_ORIGINS` mencocokkan origin secara utuh, termasuk scheme dan port. Spasi
di sekitar nilai serta entri kosong diabaikan. `*` mengizinkan semua origin;
daftar yang hanya berisi spasi atau koma tidak memberi izin ke origin mana pun.

Preflight menerima method `GET` dengan header `Content-Type` dan `Authorization`.
Preflight yang diizinkan mendapat HTTP 204; origin, method, atau header lain mendapat
HTTP 403. Request biasa dari origin yang tidak terdaftar tetap diproses tanpa header
izin CORS, sehingga browser tidak dapat membaca responsnya. CORS bukan autentikasi;
request tanpa `Origin`, misalnya dari curl, tetap bisa memakai API.

Frontend dapat membaca header respons `X-Request-ID` dan `Retry-After` untuk
melacak error dan menentukan waktu retry.

Contoh untuk frontend lokal di PowerShell:

```powershell
$env:ALLOWED_ORIGINS = "http://localhost:3000,http://localhost:5173"
$env:RATE_LIMIT_PER_MIN = "60"
$env:RATE_LIMIT_BURST = "10"
go run .
```

Restart server setelah mengubah environment variable.

### Log Request

Log akses menyertakan request ID acak, method, template route, status, dan durasi.
Query string, parameter path, alamat IP, cookie, dan header autentikasi tidak ditulis
ke log akses. Setiap respons mendapat `X-Request-ID` baru dari server; nilai dari
client diabaikan. Log panic memakai ID yang sama tanpa mencetak isi payload panic.

### Cache Jadwal dan Kalender

- `GET /jadwal/{kelas}` dan `GET /jadwal?q=...` berbagi cache untuk teks pencarian yang sama. TTL default 300 detik (5 menit).
- `GET /kalender` menggunakan TTL terpisah, default 3600 detik (1 jam).
- Cache dipisahkan berdasarkan `BASE_URL`; perubahan sumber data tidak memakai hasil dari sumber sebelumnya.

Cache menyimpan hasil parsing yang berhasil, termasuk hasil kosong. Error dan pengambilan yang dibatalkan tidak disimpan. Request bersamaan untuk data yang sama berbagi satu proses pengambilan; TTL dihitung setelah data selesai diambil. Data yang kedaluwarsa diambil ulang saat ada request berikutnya.

Parser jadwal mengenali tabel dari header Kelas, Hari, Mata Kuliah, Waktu, Ruang,
dan Dosen, termasuk jika urutan kolom berubah atau ada tabel lain pada halaman.
Tabel dengan header lengkap tanpa baris data tetap menjadi hasil kosong yang sah.
Header yang tidak lengkap, baris yang terpotong, atau hari yang tidak dikenali
menghasilkan HTTP 502 `UPSTREAM_ERROR` dan tidak disimpan ke cache.

Semua respons JSON menggunakan `Cache-Control: no-store` agar browser dan proxy
tidak menyimpan respons API, termasuk data mahasiswa dan error. Cache internal
jadwal dan kalender tetap berjalan sesuai TTL di atas.

`CACHE_ENABLED=false` menonaktifkan pembacaan dan penulisan cache. TTL nol atau negatif menonaktifkan cache untuk endpoint terkait. Konfigurasi dibaca saat startup; restart server setelah mengubah environment variable. Cache hanya ada di memori proses dan kosong kembali setelah restart. Statistik entri tersedia lewat `/health` saat cache aktif.

## FlareSolverr Setup

FlareSolverr digunakan ketika BAAK menampilkan challenge Cloudflare. Untuk development
lokal, jalankan service dari `compose.yaml`:

```bash
docker compose up -d --wait flaresolverr
```

Service memakai FlareSolverr 3.5.0 dan hanya membuka port `127.0.0.1:8191`.
Log request dinonaktifkan pada level `warning` agar cookie dan token form tidak
tercetak di log. API Go dijalankan langsung di komputer, terpisah dari container.

API membatasi dua request FlareSolverr aktif per proses secara default. Request saat
kapasitas penuh langsung mendapat HTTP 503 `FLARESOLVERR_BUSY` dengan
`Retry-After: 1`, tanpa antrean tambahan. Atur `FLARESOLVERR_MAX_CONCURRENT` sesuai
kapasitas solver; nilai tidak valid, nol, atau negatif memakai default 2. Batas ini
dibagi semua scraper dalam satu proses API, bukan lintas replica. Health check tidak
memakai slot solver, dan penolakan karena kapasitas tidak dihitung sebagai kegagalan
dependency oleh circuit breaker.

Pembatalan atau deadline request menghentikan waktu tunggu pemanggil. Pekerjaan
solver yang sudah dikirim tetap memakai slot hingga respons selesai dibaca atau
batas waktu koneksi solver tercapai (maksimal 65 detik). Hasil yang datang setelah
pemanggil batal tidak memperbarui cookie atau cache. Batas waktu koneksi membatasi
pekerjaan lokal API; terputusnya koneksi tidak menjamin browser di server solver
langsung berhenti.

Di PowerShell:

```powershell
$env:FLARESOLVERR_URL = "http://127.0.0.1:8191"
go run .
```

Untuk menghentikan FlareSolverr, jalankan `docker compose down`.

## Development

### Prasyarat

- Go 1.23 atau lebih tinggi (`go.mod` memilih toolchain 1.24.1 jika diperlukan)
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

3. Jalankan Docker Desktop dan FlareSolverr:

```bash
docker compose up -d --wait flaresolverr
```

4. Jalankan server:

```powershell
$env:FLARESOLVERR_URL = "http://127.0.0.1:8191"
go run .
```

5. Dari terminal lain, cek API:

```powershell
Invoke-RestMethod http://localhost:8080/health
Invoke-RestMethod http://localhost:8080/jadwal/1IA01
Invoke-RestMethod http://localhost:8080/kalender
```

Request pertama bisa memerlukan belasan detik untuk menyelesaikan challenge.
Request jadwal dan kalender berikutnya menggunakan cache selama TTL masih berlaku.

### Hasil Uji Live (9 September 2026)

- Pencarian `1IA01` mengembalikan 14 mata kuliah untuk semester yang tersedia di
  BAAK, Genap 2025/2026. Token diambil dari form homepage; `/jadwal` pada situs BAAK
  sendiri mengembalikan halaman 404.
- Kalender mengembalikan 17 kegiatan. Rentang tanggal dengan tanda pisah pada
  halaman BAAK dipisahkan menjadi `start` dan `end`.
- Endpoint UTS di situs BAAK mengembalikan halaman `Server Error`. API merespons
  HTTP 502 `UPSTREAM_ERROR`; ini tidak dianggap sebagai data kosong yang sukses.
- Pencarian kelas baru dan mahasiswa baru yang tidak ditemukan menghasilkan
  HTTP 404 `NOT_FOUND` setelah BAAK menampilkan pesan hasil kosong.

FlareSolverr 3.5.0 dapat melaporkan `solution.status=200` untuk halaman error.
Parser memeriksa keberadaan tabel yang dibutuhkan. Hasil kosong yang sah tetap
diterima; halaman tanpa tabel atau pesan hasil kosong dikenali sebagai error.
`/health` menunjukkan keterjangkauan FlareSolverr dan state circuit breaker,
bukan jaminan seluruh endpoint BAAK sedang menyediakan data.

### Test

```bash
go test ./...
go vet ./...
```

Test otomatis menggunakan server HTTP, HTML fixture lokal, dan PDF buatan test.
Test ekstraksi dan integrasi endpoint menjalankan Python/pdfplumber asli; install
`pdfplumber==0.11.9` dan atur `PDF_PYTHON` jika executable bukan `python3`.
Test lokal ini tidak memerlukan FlareSolverr atau akses jaringan ke BAAK.

Untuk memeriksa akses memori bersama dengan race detector di Linux:

```bash
docker build --target test -t baak-api:test .
```

CI menjalankan race detector, vet, build, serta smoke test API dan FlareSolverr
dalam image produksi. Request memiliki batas waktu 50 detik, pagination dibatasi
100 halaman, dan cache memiliki batas jumlah entri. Circuit breaker BAAK dan
FlareSolverr menyimpan kegagalan lintas request dan hanya mengizinkan satu probe
pemulihan pada satu waktu.

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

