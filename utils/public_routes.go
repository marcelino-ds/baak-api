package utils

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/yafyx/baak-api/config"
	"github.com/yafyx/baak-api/models"
)

var publicPaths = map[string]string{
	"ujian-utama": "/jadwal/cariUtama", "uas": "/jadwal/cariUas",
	"mata-kuliah": "/kuliahUjian/2", "dosen-wali": "/kuliahUjian/3",
	"koordinator": "/kuliahUjian/4", "pembimbing-pi": "/kuliahUjian/5",
	"panduan-kuliah": "/kuliahUjian/6", "jadwal-ujian": "/kuliahUjian/7",
	"ujian-bentrok": "/kuliahUjian/8", "frs": "/kuliahUjian/9",
	"daftar-ulang": "/adminAkademik/1", "cuti": "/adminAkademik/2",
	"nonaktif": "/adminAkademik/3", "pengecekan-nilai": "/adminAkademik/4",
	"pindah-lokasi": "/adminAkademik/5", "pindah-jurusan": "/adminAkademik/6",
	"berita": "/beritabaak", "buku-pedoman": "/buku_pedoman", "kalender": "/kuliahUjian/1",
	"ringkasan": "/", "situs": "/", "loket": "/", "arsip-kalender": "/",
	"dokumen-mata-kuliah": "/kuliahUjian/2", "dokumen-buku-pedoman": "/buku_pedoman",
	"dokumen-frs": "/kuliahUjian/9", "dokumen-kalender": "/", "ktm": "/",
}

// PublicCatalog describes every public, read-only section exposed by the API.
func PublicCatalog() []models.PublicSection {
	sections := []models.PublicSection{
		{ID: "ringkasan", Name: "Ringkasan BAAK", Description: "Beranda, kalender terkini, berita, dokumen, dan tautan resmi.", Endpoint: "/public/ringkasan", Methods: []string{"GET"}},
		{ID: "jadwal-kuliah", Name: "Jadwal Kuliah", Description: "Jadwal per kelas atau dosen.", Endpoint: "/jadwal/{kelas_atau_dosen}", Methods: []string{"GET"}, Parameter: "kelas_atau_dosen"},
		{ID: "kalender", Name: "Kalender Akademik", Description: "Kegiatan dan rentang tanggal akademik.", Endpoint: "/kalender", Methods: []string{"GET"}},
		{ID: "kelas-baru", Name: "Kelas Baru", Description: "Perubahan kelas mahasiswa.", Endpoint: "/kelasbaru/{kelas_atau_nama}", Methods: []string{"GET"}, Parameter: "kelas_atau_nama"},
		{ID: "mahasiswa-baru", Name: "Info Mahasiswa/KTM", Description: "Informasi mahasiswa baru dan pengambilan KTM.", Endpoint: "/mahasiswabaru/{kelas_atau_nama}", Methods: []string{"GET"}, Parameter: "kelas_atau_nama"},
		{ID: "ujian-utama", Name: "Jadwal Ujian Utama", Description: "Jadwal ujian berdasarkan jurusan.", Endpoint: "/ujian-utama?jurusan=...", Methods: []string{"GET"}, Parameter: "jurusan"},
		{ID: "uas", Name: "Jadwal UAS", Description: "Jadwal UAS berdasarkan kelas.", Endpoint: "/uas/{kelas}", Methods: []string{"GET"}, Parameter: "kelas"},
		{ID: "mata-kuliah", Name: "Daftar Mata Kuliah", Description: "Katalog mata kuliah publik.", Endpoint: "/mata-kuliah", Methods: []string{"GET"}},
		{ID: "dosen-wali", Name: "Dosen Wali Kelas", Description: "Dosen wali berdasarkan tingkat kelas.", Endpoint: "/dosen-wali/{tingkat}", Methods: []string{"GET"}, Parameter: "tingkat"},
		{ID: "koordinator", Name: "Koordinator Mata Kuliah", Description: "Koordinator mata kuliah dan kelas.", Endpoint: "/koordinator", Methods: []string{"GET"}},
		{ID: "pembimbing-pi", Name: "Pembimbing PI", Description: "Dosen pembimbing dan mahasiswa PI.", Endpoint: "/pembimbing-pi", Methods: []string{"GET"}},
		{ID: "panduan-kuliah", Name: "Panduan Jadwal Kuliah", Description: "Keterangan waktu, lokasi, dan cara membaca jadwal kuliah.", Endpoint: "/panduan-kuliah", Methods: []string{"GET"}},
		{ID: "jadwal-ujian", Name: "Panduan Jadwal Ujian", Description: "Informasi umum jadwal ujian.", Endpoint: "/jadwal-ujian", Methods: []string{"GET"}},
		{ID: "ujian-bentrok", Name: "Ujian Bentrok", Description: "Prosedur pengurusan ujian bentrok.", Endpoint: "/ujian-bentrok", Methods: []string{"GET"}},
		{ID: "frs", Name: "Formulir Rencana Studi", Description: "Panduan FRS/KRS dan dokumen terkait.", Endpoint: "/frs", Methods: []string{"GET"}},
		{ID: "berita", Name: "Berita BAAK", Description: "Daftar dan detail berita BAAK.", Endpoint: "/berita[/{id}]", Methods: []string{"GET"}, Parameter: "id"},
		{ID: "buku-pedoman", Name: "Buku Pedoman", Description: "Daftar dokumen pedoman resmi.", Endpoint: "/buku-pedoman", Methods: []string{"GET"}},
		{ID: "dokumen-teks", Name: "Teks Dokumen", Description: "Ekstraksi teks dan tabel dari dokumen PDF resmi.", Endpoint: "/dokumen/{kategori}/{id}/teks", Methods: []string{"GET"}, Parameter: "kategori,id"},
		{ID: "layanan", Name: "Layanan Administrasi", Description: "Syarat dan prosedur layanan administrasi publik.", Endpoint: "/layanan/{slug}", Methods: []string{"GET"}, Parameter: "slug"},
		{ID: "situs", Name: "Situs Resmi", Description: "Tautan RPS, sidang, dan situs jurusan.", Endpoint: "/situs", Methods: []string{"GET"}},
		{ID: "loket", Name: "Jam Loket BAAK", Description: "Jam pelayanan loket BAAK.", Endpoint: "/loket", Methods: []string{"GET"}},
	}
	return sections
}

// PublicPath resolves only known public BAAK resources. Parameters never become arbitrary URLs.
func PublicPath(section, parameter string) (string, bool) {
	section = strings.ToLower(strings.TrimSpace(section))
	parameter = strings.TrimSpace(parameter)
	base, exists := publicPaths[section]
	if !exists {
		return "", false
	}
	if parameter == "" {
		return base, true
	}
	switch section {
	case "dosen-wali":
		if len(parameter) == 1 && parameter >= "1" && parameter <= "5" {
			return base + "/" + parameter, true
		}
	case "berita":
		if publicIDPattern.MatchString(parameter) {
			return base + "/" + parameter, true
		}
	case "uas":
		if len(parameter) >= 3 && len(parameter) <= 20 && strings.IndexFunc(parameter, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
		}) < 0 {
			return base, true
		}
	}
	return "", false
}

// PublicTarget keeps public requests on the configured source and rejects traversal or query injection.
func PublicTarget(baseURL, resource string, query url.Values) (string, error) {
	if err := config.ValidateBaseURL(baseURL); err != nil {
		return "", err
	}
	valid := false
	for section := range publicPaths {
		if base, ok := PublicPath(section, ""); ok && base == resource {
			valid = true
			break
		}
		parts := strings.Split(resource, "/")
		if candidate, ok := PublicPath(section, parts[len(parts)-1]); ok && candidate == resource {
			valid = true
			break
		}
	}
	if !valid {
		return "", fmt.Errorf("invalid public resource")
	}
	target := strings.TrimRight(baseURL, "/") + resource
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	return target, nil
}
