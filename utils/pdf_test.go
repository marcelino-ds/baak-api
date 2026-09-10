package utils

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestExtractPDFTextAndPageMetadata(t *testing.T) {
	data := testTextPDF("BAAK Academic Guide")
	doc, err := ExtractPDF(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PageCount != 1 || len(doc.Pages) != 1 || !strings.Contains(doc.Pages[0].Text, "BAAK Academic Guide") {
		t.Fatalf("PDF: %+v", doc)
	}
}

func TestExtractPDFRejectsHTML(t *testing.T) {
	if _, err := ExtractPDF(context.Background(), []byte("<html>Cloudflare</html>")); err == nil {
		t.Fatal("HTML accepted as PDF")
	}
}

func testTextPDF(text string) []byte {
	return testStreamPDF("BT /F1 12 Tf 50 750 Td ("+text+") Tj ET", 1)
}

func testStreamPDF(stream string, count int) []byte {
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	var kids strings.Builder
	for i := range count {
		fmt.Fprintf(&kids, "%d 0 R ", 5+i)
	}
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), count),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
	}
	for range count {
		objects = append(objects,
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources << /Font << /F1 3 0 R >> >> /Contents 4 0 R >>")
	}
	offsets := []int{0}
	for i, obj := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return []byte(b.String())
}

func TestExtractPDFTables(t *testing.T) {
	stream := `0.5 w
50 700 m 250 700 l S 50 680 m 250 680 l S 50 660 m 250 660 l S
50 700 m 50 660 l S 150 700 m 150 660 l S 250 700 m 250 660 l S
BT /F1 12 Tf 60 685 Td (Kode) Tj ET BT /F1 12 Tf 160 685 Td (SKS) Tj ET
BT /F1 12 Tf 60 665 Td (IF101) Tj ET BT /F1 12 Tf 160 665 Td (3) Tj ET`
	doc, err := ExtractPDF(context.Background(), testStreamPDF(stream, 1))
	if err != nil {
		t.Fatal(err)
	}
	want := [][][]string{{{"Kode", "SKS"}, {"IF101", "3"}}}
	if len(doc.Pages) != 1 || !reflect.DeepEqual(doc.Pages[0].Tables, want) {
		t.Fatalf("table extraction = %+v", doc)
	}
}

func TestExtractPDFWarnsOnPageWithoutText(t *testing.T) {
	doc, err := ExtractPDF(context.Background(), testTextPDF(""))
	if err != nil || len(doc.Warnings) != 1 || !strings.Contains(doc.Warnings[0], "OCR") {
		t.Fatalf("textless PDF = %+v, error = %v", doc, err)
	}
}

func TestExtractPDFRejectsTooManyPages(t *testing.T) {
	if _, err := ExtractPDF(context.Background(), testStreamPDF("", 101)); !errors.Is(err, ErrPDFInvalid) {
		t.Fatalf("101 pages: error = %v", err)
	}
}

func TestExtractPDFRuntimeMissing(t *testing.T) {
	t.Setenv("PDF_PYTHON", filepath.Join(t.TempDir(), "missing-python"))
	if _, err := ExtractPDF(context.Background(), testTextPDF("BAAK")); !errors.Is(err, ErrPDFUnavailable) {
		t.Fatalf("missing runtime: error = %v", err)
	}
}

// Worker fixtures exercise subprocess failures independently of PDF parser behavior.
func withPDFWorker(t *testing.T, script string) {
	t.Helper()
	previous := pdfWorker
	pdfWorker = script
	t.Cleanup(func() { pdfWorker = previous })
}

func TestExtractPDFOutputLimitIsUnsupportedDocument(t *testing.T) {
	withPDFWorker(t, `import json, sys
json.dump({"page_count": 1, "pages": [{"number": 1, "text": "x" * (5 * 1024 * 1024), "tables": []}], "warnings": []}, sys.stdout)`)
	if _, err := ExtractPDF(context.Background(), testTextPDF("BAAK")); !errors.Is(err, ErrPDFInvalid) {
		t.Fatalf("excessive output: error = %v", err)
	}
}

func TestExtractPDFMissingDependency(t *testing.T) {
	withPDFWorker(t, "raise SystemExit(3)")
	if _, err := ExtractPDF(context.Background(), testTextPDF("BAAK")); !errors.Is(err, ErrPDFUnavailable) {
		t.Fatalf("missing parser: error = %v", err)
	}
}

func TestExtractPDFTimeoutReleasesCapacity(t *testing.T) {
	withPDFWorker(t, "import time\ntime.sleep(30)")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := ExtractPDF(ctx, testTextPDF("BAAK")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("slow worker: error = %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("worker was not terminated promptly")
	}
	if len(pdfSlots) != 0 {
		t.Fatal("timed out worker retained extraction capacity")
	}
}

func TestExtractPDFRejectsThirdConcurrentWorker(t *testing.T) {
	withPDFWorker(t, "import time\ntime.sleep(30)")
	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan error, 2)
	defer func() {
		cancel()
		for range 2 {
			select {
			case err := <-results:
				if !errors.Is(err, context.Canceled) {
					t.Errorf("canceled worker: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Error("worker did not release on cancellation")
			}
		}
	}()
	for range 2 {
		go func() {
			_, err := ExtractPDF(ctx, testTextPDF("BAAK"))
			results <- err
		}()
	}
	deadline := time.Now().Add(3 * time.Second)
	for len(pdfSlots) != 2 {
		if time.Now().After(deadline) {
			t.Fatal("workers did not start")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := ExtractPDF(context.Background(), testTextPDF("BAAK")); !errors.Is(err, ErrPDFBusy) {
		t.Fatalf("third extraction = %v", err)
	}
}
