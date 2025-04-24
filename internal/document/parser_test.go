package document

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/jung-kurt/gofpdf"
)

func createTempFile(t *testing.T, content, ext string) string {
	tmpFile, err := ioutil.TempFile("", "docqa-test-*"+ext)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	if _, err := tmpFile.Write([]byte(content)); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile.Close()
	return tmpFile.Name()
}

func createTempPDF(t *testing.T, text string) string {
	tmpFile, err := ioutil.TempFile("", "docqa-test-*.pdf")
	if err != nil {
		t.Fatalf("Failed to create temp PDF file: %v", err)
	}
	defer tmpFile.Close()

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "", 12)
	pdf.MultiCell(0, 10, text, "", "", false)
	if err := pdf.Output(tmpFile); err != nil {
		t.Fatalf("Failed to write PDF: %v", err)
	}
	return tmpFile.Name()
}

func TestPlainTextParser(t *testing.T) {
	content := "Hello, this is a plain text file.\nSecond line."
	file := createTempFile(t, content, ".txt")
	defer os.Remove(file)

	parser := NewPlainTextParser()
	text, err := parser.Parse(file)
	if err != nil {
		t.Fatalf("PlainTextParser.Parse failed: %v", err)
	}
	if !strings.Contains(text, "plain text file") {
		t.Errorf("Expected content not found in parsed text: %s", text)
	}
}

func TestMarkdownParser(t *testing.T) {
	content := "# Title\n\nThis is a **markdown** file.\n\n- Item 1\n- Item 2"
	file := createTempFile(t, content, ".md")
	defer os.Remove(file)

	parser := NewMarkdownParser()
	text, err := parser.Parse(file)
	if err != nil {
		t.Fatalf("MarkdownParser.Parse failed: %v", err)
	}
	if !strings.Contains(text, "markdown file") {
		t.Errorf("Expected content not found in parsed text: %s", text)
	}
	if !strings.Contains(text, "Item 1") {
		t.Errorf("Expected list item not found in parsed text: %s", text)
	}
}

func TestPDFParser(t *testing.T) {
	content := "This is a PDF test.\nSecond line."
	file := createTempPDF(t, content)
	defer os.Remove(file)

	parser := NewPDFParser()
	text, err := parser.Parse(file)
	if err != nil {
		t.Fatalf("PDFParser.Parse failed: %v", err)
	}
	if !strings.Contains(text, "PDF test") {
		t.Errorf("Expected content not found in parsed PDF text: %s", text)
	}
}

func TestParserFactory(t *testing.T) {
	txtFile := createTempFile(t, "plain text", ".txt")
	defer os.Remove(txtFile)
	mdFile := createTempFile(t, "# Markdown", ".md")
	defer os.Remove(mdFile)
	pdfFile := createTempPDF(t, "PDF content")
	defer os.Remove(pdfFile)

	tests := []struct {
		file     string
		expected string
	}{
		{txtFile, "plain text"},
		{mdFile, "Markdown"},
		{pdfFile, "PDF content"},
	}

	for _, tt := range tests {
		parser, err := ParserFactory(tt.file)
		if err != nil {
			t.Fatalf("ParserFactory failed for %s: %v", tt.file, err)
		}
		text, err := parser.Parse(tt.file)
		if err != nil {
			t.Fatalf("Parser.Parse failed for %s: %v", tt.file, err)
		}
		if !strings.Contains(text, tt.expected) {
			t.Errorf("Expected '%s' in parsed text, got: %s", tt.expected, text)
		}
	}
}
