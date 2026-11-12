// Package export writes the ranked-candidates workbook HR downloads.
//
// The desktop agent used the excelize library; this is a small standard-
// library writer producing the same three sheets (Summary, Ranked
// Candidates, Detailed Analysis) with colour-coded scores.
package export

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Cell styles (indexes into cellXfs in styles.xml).
const (
	StyleDefault = 0
	StyleHeader  = 1
	StyleGreen   = 2 // score >= 70
	StyleAmber   = 3 // 55-69.9
	StyleRed     = 4 // < 55
	StyleWrap    = 5
	StyleTitle   = 6
	StyleNumber  = 7
	StyleBold    = 8
)

// Cell is one value. V is string, float64, int or nil.
type Cell struct {
	V     any
	Style int
}

// Sheet is one worksheet.
type Sheet struct {
	Name       string
	ColWidths  []float64
	Rows       [][]Cell
	FreezeRows int
}

// ScoreStyle picks the colour for a total score.
func ScoreStyle(total float64) int {
	switch {
	case total >= 70:
		return StyleGreen
	case total >= 55:
		return StyleAmber
	}
	return StyleRed
}

// Write produces an .xlsx file.
func Write(w io.Writer, sheets []Sheet) error {
	zw := zip.NewWriter(w)
	files := map[string]string{
		"[Content_Types].xml":        contentTypes(len(sheets)),
		"_rels/.rels":                rootRels,
		"xl/workbook.xml":            workbook(sheets),
		"xl/_rels/workbook.xml.rels": workbookRels(len(sheets)),
		"xl/styles.xml":              styles,
	}
	order := []string{"[Content_Types].xml", "_rels/.rels", "xl/workbook.xml", "xl/_rels/workbook.xml.rels", "xl/styles.xml"}
	for i, s := range sheets {
		name := fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1)
		files[name] = sheetXML(s)
		order = append(order, name)
	}
	for _, name := range order {
		f, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(f, files[name]); err != nil {
			return err
		}
	}
	return zw.Close()
}

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(cleanXML(s)))
	return b.String()
}

// cleanXML drops characters that are illegal in XML 1.0.
func cleanXML(s string) string {
	return strings.Map(func(r rune) rune {
		if r == 0x9 || r == 0xA || r == 0xD || (r >= 0x20 && r <= 0xD7FF) || (r >= 0xE000 && r <= 0xFFFD) || r >= 0x10000 {
			return r
		}
		return -1
	}, s)
}

func colName(i int) string {
	name := ""
	for i++; i > 0; i = (i - 1) / 26 {
		name = string(rune('A'+(i-1)%26)) + name
	}
	return name
}

func sheetXML(s Sheet) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`)
	if s.FreezeRows > 0 {
		fmt.Fprintf(&b, `<sheetViews><sheetView workbookViewId="0"><pane ySplit="%d" topLeftCell="A%d" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>`, s.FreezeRows, s.FreezeRows+1)
	}
	if len(s.ColWidths) > 0 {
		b.WriteString("<cols>")
		for i, w := range s.ColWidths {
			fmt.Fprintf(&b, `<col min="%d" max="%d" width="%.1f" customWidth="1"/>`, i+1, i+1, w)
		}
		b.WriteString("</cols>")
	}
	b.WriteString("<sheetData>")
	for r, row := range s.Rows {
		fmt.Fprintf(&b, `<row r="%d">`, r+1)
		for c, cell := range row {
			ref := colName(c) + strconv.Itoa(r+1)
			style := ""
			if cell.Style != 0 {
				style = fmt.Sprintf(` s="%d"`, cell.Style)
			}
			switch v := cell.V.(type) {
			case nil:
				if cell.Style != 0 {
					fmt.Fprintf(&b, `<c r="%s"%s/>`, ref, style)
				}
			case float64:
				fmt.Fprintf(&b, `<c r="%s"%s><v>%s</v></c>`, ref, style, strconv.FormatFloat(v, 'f', -1, 64))
			case int:
				fmt.Fprintf(&b, `<c r="%s"%s><v>%d</v></c>`, ref, style, v)
			case string:
				fmt.Fprintf(&b, `<c r="%s"%s t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, ref, style, esc(v))
			default:
				fmt.Fprintf(&b, `<c r="%s"%s t="inlineStr"><is><t>%s</t></is></c>`, ref, style, esc(fmt.Sprint(v)))
			}
		}
		b.WriteString("</row>")
	}
	b.WriteString("</sheetData></worksheet>")
	return b.String()
}

func contentTypes(n int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	b.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/>`)
	b.WriteString(`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`)
	b.WriteString(`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>`)
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, `<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, i)
	}
	b.WriteString(`</Types>`)
	return b.String()
}

const rootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`

func workbook(sheets []Sheet) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>`)
	for i, s := range sheets {
		name := s.Name
		for _, bad := range []string{"\\", "/", "?", "*", "[", "]", ":"} {
			name = strings.ReplaceAll(name, bad, " ")
		}
		if len(name) > 31 {
			name = name[:31]
		}
		fmt.Fprintf(&b, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, esc(name), i+1, i+1)
	}
	b.WriteString(`</sheets></workbook>`)
	return b.String()
}

func workbookRels(n int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, i, i)
	}
	fmt.Fprintf(&b, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`, n+1)
	b.WriteString(`</Relationships>`)
	return b.String()
}

const styles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<numFmts count="1"><numFmt numFmtId="164" formatCode="0.0"/></numFmts>
<fonts count="3">
 <font><sz val="11"/><name val="Calibri"/></font>
 <font><b/><sz val="11"/><color rgb="FFFFFFFF"/><name val="Calibri"/></font>
 <font><b/><sz val="14"/><name val="Calibri"/></font>
</fonts>
<fills count="6">
 <fill><patternFill patternType="none"/></fill>
 <fill><patternFill patternType="gray125"/></fill>
 <fill><patternFill patternType="solid"><fgColor rgb="FF1F3A5F"/></patternFill></fill>
 <fill><patternFill patternType="solid"><fgColor rgb="FFC6EFCE"/></patternFill></fill>
 <fill><patternFill patternType="solid"><fgColor rgb="FFFFEB9C"/></patternFill></fill>
 <fill><patternFill patternType="solid"><fgColor rgb="FFFFC7CE"/></patternFill></fill>
</fills>
<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>
<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
<cellXfs count="9">
 <xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>
 <xf numFmtId="0" fontId="1" fillId="2" borderId="0" xfId="0" applyFont="1" applyFill="1"><alignment vertical="center" wrapText="1"/></xf>
 <xf numFmtId="164" fontId="0" fillId="3" borderId="0" xfId="0" applyNumberFormat="1" applyFill="1"/>
 <xf numFmtId="164" fontId="0" fillId="4" borderId="0" xfId="0" applyNumberFormat="1" applyFill="1"/>
 <xf numFmtId="164" fontId="0" fillId="5" borderId="0" xfId="0" applyNumberFormat="1" applyFill="1"/>
 <xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0" applyAlignment="1"><alignment vertical="top" wrapText="1"/></xf>
 <xf numFmtId="0" fontId="2" fillId="0" borderId="0" xfId="0" applyFont="1"/>
 <xf numFmtId="164" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>
 <xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0" applyFont="1"/>
</cellXfs>
<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>
</styleSheet>`
