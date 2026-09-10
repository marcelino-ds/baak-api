package utils

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/yafyx/baak-api/models"
)

//go:embed pdf_worker.py
var pdfWorker string

var (
	ErrPDFUnavailable = errors.New("PDF extraction runtime unavailable")
	ErrPDFInvalid     = errors.New("PDF is invalid, encrypted, or exceeds extraction limits")
	ErrPDFBusy        = errors.New("PDF extraction capacity is full")
	pdfSlots          = make(chan struct{}, 2)
)

// Do not embed bytes.Buffer: its ReadFrom would let io.Copy bypass Write's limit.
type limitedPDFOutput struct{ buffer bytes.Buffer }

func (b *limitedPDFOutput) Write(p []byte) (int, error) {
	if b.buffer.Len()+len(p) > 4<<20 {
		return 0, ErrPDFInvalid
	}
	return b.buffer.Write(p)
}

// ExtractPDF runs the parser in a bounded subprocess; PDF content is never executed as code.
func ExtractPDF(ctx context.Context, data []byte) (models.PDFDocument, error) {
	var result models.PDFDocument
	if !bytes.HasPrefix(data, []byte("%PDF-")) || len(data) > 16<<20 {
		return result, ErrPDFInvalid
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	select {
	case pdfSlots <- struct{}{}:
		defer func() { <-pdfSlots }()
	default:
		return result, ErrPDFBusy
	}
	python := os.Getenv("PDF_PYTHON")
	if python == "" {
		python = "python3"
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-I", "-c", pdfWorker)
	cmd.Stdin = bytes.NewReader(data)
	var output limitedPDFOutput
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if errors.Is(err, ErrPDFInvalid) {
			return result, ErrPDFInvalid
		}
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() == 3 {
			return result, ErrPDFUnavailable
		}
		return result, ErrPDFInvalid
	}
	if err := json.Unmarshal(output.buffer.Bytes(), &result); err != nil {
		return result, ErrPDFInvalid
	}
	return result, nil
}
