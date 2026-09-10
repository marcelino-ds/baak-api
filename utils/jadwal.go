package utils

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type jadwalTable struct {
	rows    *goquery.Selection
	columns map[string]int
}

func findJadwalTable(doc *goquery.Document) (jadwalTable, error) {
	var result jadwalTable
	doc.Find("table").EachWithBreak(func(_ int, table *goquery.Selection) bool {
		// Layout tables may contain the result table; only inspect each table's own rows.
		rows := table.Find("tr").FilterFunction(func(_ int, row *goquery.Selection) bool {
			return row.Closest("table").IsSelection(table)
		})
		rows.EachWithBreak(func(index int, row *goquery.Selection) bool {
			columns := jadwalColumns(row.ChildrenFiltered("th, td"))
			if len(columns) != 6 {
				return true
			}
			result = jadwalTable{rows: rows.Slice(index+1, rows.Length()), columns: columns}
			return false
		})
		return result.rows == nil
	})
	if result.rows == nil {
		return result, fmt.Errorf("%w: schedule column headers are missing", ErrUnexpectedPage)
	}
	return result, nil
}

func jadwalColumns(cells *goquery.Selection) map[string]int {
	columns := make(map[string]int, 6)
	duplicate := false
	cells.Each(func(index int, cell *goquery.Selection) {
		name := strings.ToUpper(strings.Join(strings.Fields(cell.Text()), ""))
		switch name {
		case "KELAS", "HARI", "MATAKULIAH", "WAKTU", "RUANG", "DOSEN":
			if _, exists := columns[name]; exists {
				duplicate = true
			}
			columns[name] = index
		}
	})
	if duplicate {
		return map[string]int{}
	}
	return columns
}
