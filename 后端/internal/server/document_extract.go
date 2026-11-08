package server

import (
	"archive/zip"
	"encoding/xml"
	"io"
	"strings"
)

const docxMIME = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

func docxText(path string) (string, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer archive.Close()
	for _, file := range archive.File {
		if file.Name != "word/document.xml" {
			continue
		}
		if file.UncompressedSize64 > 8<<20 {
			return "", apiError{422, "Word 正文过大，请拆分后上传"}
		}
		body, err := file.Open()
		if err != nil {
			return "", err
		}
		defer body.Close()
		reader := xml.NewDecoder(io.LimitReader(body, 8<<20))
		var out strings.Builder
		for {
			token, err := reader.Token()
			if err == io.EOF {
				return cut(strings.TrimSpace(out.String()), 50000), nil
			}
			if err != nil {
				return "", err
			}
			switch element := token.(type) {
			case xml.StartElement:
				switch element.Name.Local {
				case "t":
					var text string
					if err := reader.DecodeElement(&text, &element); err != nil {
						return "", err
					}
					out.WriteString(text)
				case "br":
					out.WriteByte('\n')
				case "tab":
					out.WriteByte('\t')
				}
			case xml.EndElement:
				if element.Name.Local == "p" {
					out.WriteByte('\n')
				}
			}
		}
	}
	return "", apiError{422, "Word 文件没有可提取的正文"}
}
