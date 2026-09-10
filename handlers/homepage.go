package handlers

import (
	"net/http"

	"github.com/yafyx/baak-api/utils"
)

func HandlerHomepage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	endpoints := []string{
		"/live", "/ready", "/health", "/catalog", "/public/{section}",
		"/jadwal?q={kelas/dosen}", "/jadwal/{kelas}", "/kalender",
		"/kelasbaru/{kelas/npm/nama}", "/uts/{kelas/dosen}",
		"/mahasiswabaru/{kelas/nama}", "/ujian-utama?jurusan={jurusan}",
		"/uas/{kelas}", "/mata-kuliah", "/dosen-wali/{tingkat}",
		"/koordinator", "/pembimbing-pi", "/jadwal-ujian", "/ujian-bentrok",
		"/frs", "/berita/{id}", "/buku-pedoman", "/layanan/{slug}",
		"/daftar-ulang", "/cuti", "/nonaktif", "/pengecekan-nilai", "/pindah-lokasi", "/pindah-jurusan",
		"/situs", "/loket",
	}
	utils.WriteJSONResponse(w, endpoints)
}
