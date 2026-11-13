package export

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestWriteProducesWellFormedWorkbook(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, []Sheet{{Name: "Ranked: Candidates/1", FreezeRows: 1, ColWidths: []float64{10},
		Rows: [][]Cell{{{V: "Name", Style: StyleHeader}, {V: "Score"}}, {{V: "A & B <x>\x01"}, {V: 72.5, Style: ScoreStyle(72.5)}}}}})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		dec := xml.NewDecoder(bytes.NewReader(data))
		for {
			if _, err := dec.Token(); err == io.EOF {
				break
			} else if err != nil {
				t.Fatalf("%s is not well-formed XML: %v", f.Name, err)
			}
		}
		if f.Name == "xl/worksheets/sheet1.xml" {
			found++
			if !strings.Contains(string(data), "A &amp; B &lt;x&gt;") || !strings.Contains(string(data), "<v>72.5</v>") {
				t.Fatalf("sheet content wrong: %s", data)
			}
		}
	}
	if found != 1 {
		t.Fatal("sheet1 missing")
	}
	if colName(0) != "A" || colName(25) != "Z" || colName(26) != "AA" {
		t.Fatal("column names wrong")
	}
}
