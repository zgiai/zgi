package excelimport

import "fmt"

func RecommendedSheet(workbook *ParsedWorkbook) (*ParsedSheet, error) {
	if workbook == nil || len(workbook.Sheets) == 0 {
		return nil, fmt.Errorf("Excel file has no sheets")
	}
	for i := range workbook.Sheets {
		if workbook.Sheets[i].Recommended {
			return &workbook.Sheets[i], nil
		}
	}
	return &workbook.Sheets[0], nil
}
