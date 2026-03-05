package ingest

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func extractText(ctx context.Context, mime, filename string, body io.Reader) (string, error) {
	rawBytes, err := io.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	extension := strings.ToLower(filepath.Ext(filename))
	lowerMIME := strings.ToLower(strings.TrimSpace(mime))

	switch {
	case lowerMIME == "application/pdf" || extension == ".pdf":
		return extractPDFText(ctx, rawBytes)
	case strings.Contains(lowerMIME, "officedocument.wordprocessingml.document") || extension == ".docx":
		return extractDOCXText(rawBytes)
	case strings.HasPrefix(lowerMIME, "text/"), extension == ".md", extension == ".txt":
		return string(rawBytes), nil
	default:
		return "", fmt.Errorf("unsupported file type: %s", mime)
	}
}

func extractPDFText(ctx context.Context, rawBytes []byte) (string, error) {
	temporaryDir, err := os.MkdirTemp("", "vertex-rag-pdf-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(temporaryDir)

	inputPath := filepath.Join(temporaryDir, "input.pdf")
	outputPath := filepath.Join(temporaryDir, "output.txt")

	if err := os.WriteFile(inputPath, rawBytes, 0o600); err != nil {
		return "", fmt.Errorf("write temporary pdf: %w", err)
	}

	command := exec.CommandContext(ctx, "pdftotext", "-layout", "-enc", "UTF-8", inputPath, outputPath)
	if output, err := command.CombinedOutput(); err != nil {
		return "", fmt.Errorf("pdftotext failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}

	extractedText, err := os.ReadFile(outputPath)
	if err != nil {
		return "", fmt.Errorf("read extracted text: %w", err)
	}

	return string(extractedText), nil
}

func extractDOCXText(rawBytes []byte) (string, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(rawBytes), int64(len(rawBytes)))
	if err != nil {
		return "", fmt.Errorf("open docx zip: %w", err)
	}

	var documentXML []byte
	for _, file := range zipReader.File {
		if file.Name != "word/document.xml" {
			continue
		}

		reader, err := file.Open()
		if err != nil {
			return "", fmt.Errorf("open document.xml: %w", err)
		}

		documentXML, err = io.ReadAll(reader)
		reader.Close()
		if err != nil {
			return "", fmt.Errorf("read document.xml: %w", err)
		}
		break
	}

	if len(documentXML) == 0 {
		return "", errors.New("document.xml not found in docx file")
	}

	decoder := xml.NewDecoder(bytes.NewReader(documentXML))
	var builder strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", fmt.Errorf("parse document.xml: %w", err)
		}

		switch node := token.(type) {
		case xml.StartElement:
			if node.Name.Local == "t" {
				var value string
				if err := decoder.DecodeElement(&value, &node); err != nil {
					return "", fmt.Errorf("decode text node: %w", err)
				}
				text := html.UnescapeString(value)
				if text == "" {
					continue
				}
				text = strings.ReplaceAll(text, "\r", " ")
				text = strings.ReplaceAll(text, "\n", " ")
				text = strings.ReplaceAll(text, "\t", " ")
				text = strings.ReplaceAll(text, "\u00a0", " ")
				builder.WriteString(text)
			}
		case xml.EndElement:
			// Paragraph boundary: add a blank line to preserve structure for chunking.
			if node.Name.Local == "p" {
				builder.WriteString("\n\n")
			}
		}
	}

	return strings.TrimSpace(builder.String()), nil
}
