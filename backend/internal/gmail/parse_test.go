package gmail

import (
	"encoding/base64"
	"testing"

	gmailapi "google.golang.org/api/gmail/v1"
)

func TestHeaderValue(t *testing.T) {
	headers := []*gmailapi.MessagePartHeader{{Name: "Subject", Value: "Hello from Gmail"}}

	if got := HeaderValue(headers, "subject"); got != "Hello from Gmail" {
		t.Fatalf("HeaderValue() = %q, want %q", got, "Hello from Gmail")
	}

	if got := HeaderValue(headers, "Missing"); got != "" {
		t.Fatalf("HeaderValue() = %q, want empty", got)
	}
}

func TestExtractMessageBody(t *testing.T) {
	t.Run("plain text part with URL-safe Gmail encoding", func(t *testing.T) {
		payload := &gmailapi.MessagePart{
			MimeType: "multipart/alternative",
			Parts: []*gmailapi.MessagePart{{
				MimeType: "text/plain",
				Body: &gmailapi.MessagePartBody{
					Data: base64.RawURLEncoding.EncodeToString([]byte("hello world")),
				},
			}},
		}

		if got := ExtractMessageBody(payload); got != "hello world" {
			t.Fatalf("ExtractMessageBody() = %q, want %q", got, "hello world")
		}
	})

	t.Run("nested multipart mixed with alternative text/plain", func(t *testing.T) {
		payload := &gmailapi.MessagePart{
			MimeType: "multipart/mixed",
			Parts: []*gmailapi.MessagePart{{
				MimeType: "multipart/alternative",
				Parts: []*gmailapi.MessagePart{{
					MimeType: "text/plain",
					Body: &gmailapi.MessagePartBody{
						Data: base64.RawURLEncoding.EncodeToString([]byte("nested hello")),
					},
				}},
			}},
		}

		if got := ExtractMessageBody(payload); got != "nested hello" {
			t.Fatalf("ExtractMessageBody() = %q, want %q", got, "nested hello")
		}
	})

	t.Run("html-only nested part gets stripped", func(t *testing.T) {
		payload := &gmailapi.MessagePart{
			MimeType: "multipart/mixed",
			Parts: []*gmailapi.MessagePart{{
				MimeType: "multipart/alternative",
				Parts: []*gmailapi.MessagePart{{
					MimeType: "text/html",
					Body: &gmailapi.MessagePartBody{
						Data: base64.RawURLEncoding.EncodeToString([]byte("<html><body><b>Hello</b> <i>world</i></body></html>")),
					},
				}},
			}},
		}

		if got := ExtractMessageBody(payload); got != "Hello world" {
			t.Fatalf("ExtractMessageBody() = %q, want %q", got, "Hello world")
		}
	})

	t.Run("body nil does not panic", func(t *testing.T) {
		payload := &gmailapi.MessagePart{
			MimeType: "multipart/alternative",
			Parts: []*gmailapi.MessagePart{{
				MimeType: "text/plain",
			}},
		}

		if got := ExtractMessageBody(payload); got != "" {
			t.Fatalf("ExtractMessageBody() = %q, want empty", got)
		}
	})
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple tag", input: "<div>Hello <b>world</b>!</div>", want: "Hello world!"},
		{name: "nested tags", input: "<p><span>one</span> <em>two</em></p>", want: "one two"},
		{name: "empty", input: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripHTML(tt.input); got != tt.want {
				t.Fatalf("StripHTML() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecodeGmailBody(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "standard base64", input: base64.RawURLEncoding.EncodeToString([]byte("Hello from Gmail")), want: "Hello from Gmail"},
		{name: "gmail url-safe alphabet", input: base64.RawURLEncoding.EncodeToString([]byte{0xff, 0xff, 0xff, 0xff}), want: string([]byte{0xff, 0xff, 0xff, 0xff})},
		{name: "empty", input: "", want: ""},
		{name: "quoted printable", input: base64.RawURLEncoding.EncodeToString([]byte("=E2=80=99")), want: "’"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DecodeGmailBody(tt.input); got != tt.want {
				t.Fatalf("DecodeGmailBody() = %q, want %q", got, tt.want)
			}
		})
	}
}
