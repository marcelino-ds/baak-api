package utils

import (
	"fmt"

	"github.com/yafyx/baak-api/models"
)

// ValidatePublicTables prevents unrelated tables or search forms from becoming cached results.
func ValidatePublicTables(page *models.PublicPage, section string) error {
	var required []string
	switch section {
	case "uas":
		required = []string{"hari", "tanggal", "mata_kuliah", "waktu"}
	case "ujian-utama":
		required = []string{"hari", "tanggal", "mata_kuliah", "waktu", "ruang"}
	case "dosen-wali":
		required = []string{"kelas", "dosen"}
	case "koordinator":
		required = []string{"mata_kuliah", "kelas", "dosen"}
	case "pembimbing-pi":
		required = []string{"kelas", "kelompok", "npm", "nama_mhs", "dosen_pembimbing"}
	default:
		return nil
	}
	matched := make([]models.PublicTable, 0)
	for _, table := range page.Tables {
		keys := make(map[string]bool, len(table.Headers))
		for _, header := range table.Headers {
			keys[publicColumnKey(header)] = true
		}
		valid := true
		for _, key := range required {
			if !keys[key] {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		for _, row := range table.Rows {
			if len(row) != len(table.Headers) {
				return fmt.Errorf("%w: %s result has incomplete columns", ErrUnexpectedPage, section)
			}
		}
		matched = append(matched, table)
	}
	if len(matched) == 0 {
		return fmt.Errorf("%w: %s result table is missing", ErrUnexpectedPage, section)
	}
	page.Tables = matched
	return nil
}
