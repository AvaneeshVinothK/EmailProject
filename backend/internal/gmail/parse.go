package gmail

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime/quotedprintable"
	"strings"
	"unicode/utf8"

	gmailapi "google.golang.org/api/gmail/v1"
)

func HeaderValue(headers []*gmailapi.MessagePartHeader, name string) string {
	for _, h := range headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

func ExtractMessageBody(payload *gmailapi.MessagePart) string {
	if payload == nil {
		return ""
	}

	if payload.Body != nil && payload.Body.Data != "" {
		decoded := DecodeGmailBody(payload.Body.Data)
		if decoded == "" {
			return ""
		}
		if payload.MimeType == "text/html" {
			return StripHTML(decoded)
		}
		return decoded
	}

	for _, part := range payload.Parts {
		if part.MimeType == "text/plain" {
			if part.Body != nil && part.Body.Data != "" {
				if body := DecodeGmailBody(part.Body.Data); body != "" {
					return body
				}
			}
		}
	}

	for _, part := range payload.Parts {
		if part.MimeType == "text/html" {
			if part.Body != nil && part.Body.Data != "" {
				if body := DecodeGmailBody(part.Body.Data); body != "" {
					return StripHTML(body)
				}
			}
		}
	}

	for _, part := range payload.Parts {
		if body := ExtractMessageBody(part); body != "" {
			return body
		}
	}

	return ""
}

func StripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}

	text := strings.Join(strings.Fields(b.String()), " ")
	return strings.TrimSpace(text)
}

func DecodeGmailBody(data string) string {
	if strings.TrimSpace(data) == "" {
		return ""
	}

	payload := strings.ReplaceAll(data, "-", "+")
	payload = strings.ReplaceAll(payload, "_", "/")
	if remainder := len(payload) % 4; remainder > 0 {
		payload += strings.Repeat("=", 4-remainder)
	}

	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(payload)
		if err != nil {
			return data
		}
	}

	text := decoded
	if !utf8.Valid(text) {
		return string(decoded)
	}

	reader := quotedprintable.NewReader(bytes.NewReader(decoded))
	qpBody, err := io.ReadAll(reader)
	if err == nil {
		text = qpBody
	}

	if utf8.Valid(text) {
		return strings.TrimSpace(string(text))
	}

	return strings.TrimSpace(string(decoded))
}
