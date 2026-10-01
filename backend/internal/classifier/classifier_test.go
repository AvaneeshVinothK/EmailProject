package classifier

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AvaneeshVinothK/EmailProject/internal/models"
	"github.com/AvaneeshVinothK/EmailProject/internal/throttle"
)

func TestClassifier_ClassifySuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer test-key"; got != want {
			t.Fatalf("Authorization header = %q, want %q", got, want)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("json.Unmarshal() error: %v", err)
		}
		responseFormat, ok := payload["response_format"].(map[string]any)
		if !ok {
			t.Fatalf("response_format missing: %v", payload)
		}
		jsonSchema, ok := responseFormat["json_schema"].(map[string]any)
		if !ok {
			t.Fatalf("json_schema missing: %v", responseFormat)
		}
		if got, want := jsonSchema["strict"], true; got != want {
			t.Fatalf("strict = %v, want %v", got, want)
		}
		schema, ok := jsonSchema["schema"].(map[string]any)
		if !ok {
			t.Fatalf("schema missing: %v", jsonSchema)
		}
		if got, want := schema["additionalProperties"], false; got != want {
			t.Fatalf("additionalProperties = %v, want %v", got, want)
		}

		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"category\":\"next_steps\",\"confidence\":0.84}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	oldURL := cerebrasAPIURL
	cerebrasAPIURL = server.URL + "/v1/chat/completions"
	defer func() { cerebrasAPIURL = oldURL }()

	t.Setenv("CEREBRAS_API_KEY", "test-key")
	t.Setenv("CEREBRAS_MODEL", "test-model")

	c, err := New(context.Background())
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	result, err := c.Classify(context.Background(), "Subject", "sender@example.com", "Hello there", []Category{{Name: "next_steps", Description: "Follow-up"}, {Name: "other", Description: "Everything else"}})
	if err != nil {
		t.Fatalf("Classify() returned error: %v", err)
	}
	if result.Category != "next_steps" {
		t.Fatalf("Category = %q, want %q", result.Category, "next_steps")
	}
	if result.Confidence != 0.84 {
		t.Fatalf("Confidence = %v, want 0.84", result.Confidence)
	}
}

func TestClassifier_ClassifyRejectsCategoryNotInList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"category\":\"unknown\",\"confidence\":0.9}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	oldURL := cerebrasAPIURL
	cerebrasAPIURL = server.URL + "/v1/chat/completions"
	defer func() { cerebrasAPIURL = oldURL }()

	t.Setenv("CEREBRAS_API_KEY", "test-key")
	c, err := New(context.Background())
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	_, err = c.Classify(context.Background(), "Subject", "sender@example.com", "Hello", []Category{{Name: "next_steps", Description: "Follow-up"}, {Name: "other", Description: "Everything else"}})
	if err == nil || !strings.Contains(err.Error(), "invalid category") {
		t.Fatalf("Classify() error = %v, want invalid category error", err)
	}
}

func TestClassifier_ClassifyRejectsEmptyContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""},"finish_reason":"length"}]}`))
	}))
	defer server.Close()

	oldURL := cerebrasAPIURL
	cerebrasAPIURL = server.URL + "/v1/chat/completions"
	defer func() { cerebrasAPIURL = oldURL }()

	t.Setenv("CEREBRAS_API_KEY", "test-key")
	c, err := New(context.Background())
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	_, err = c.Classify(context.Background(), "Subject", "sender@example.com", "Hello", []Category{{Name: "next_steps", Description: "Follow-up"}, {Name: "other", Description: "Everything else"}})
	if err == nil || !strings.Contains(err.Error(), "empty content") || !strings.Contains(err.Error(), "length") {
		t.Fatalf("Classify() error = %v, want empty content and finish reason", err)
	}
}

func TestClassifier_RequestBodyUsesStrictJSONSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error: %v", err)
		}
		payload := string(body)
		if !strings.Contains(payload, `"strict":true`) {
			t.Fatalf("request body missing strict=true: %s", payload)
		}
		if !strings.Contains(payload, `"additionalProperties":false`) {
			t.Fatalf("request body missing additionalProperties=false: %s", payload)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"category\":\"next_steps\",\"confidence\":0.5}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	oldURL := cerebrasAPIURL
	cerebrasAPIURL = server.URL + "/v1/chat/completions"
	defer func() { cerebrasAPIURL = oldURL }()

	t.Setenv("CEREBRAS_API_KEY", "test-key")
	c, err := New(context.Background())
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	_, err = c.Classify(context.Background(), "Subject", "sender@example.com", "Hello", []Category{{Name: "next_steps", Description: "Follow-up"}, {Name: "other", Description: "Everything else"}})
	if err != nil {
		t.Fatalf("Classify() returned error: %v", err)
	}
}

func TestClassifier_ClassifyRetriesOn503(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error: %v", err)
		}
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"temporarily unavailable"}`))
			return
		}
		if string(body) == "" {
			t.Fatalf("request body was empty on attempt %d", calls)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"category\":\"next_steps\",\"confidence\":0.75}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	oldURL := cerebrasAPIURL
	cerebrasAPIURL = server.URL + "/v1/chat/completions"
	defer func() { cerebrasAPIURL = oldURL }()

	t.Setenv("CEREBRAS_API_KEY", "test-key")
	c, err := New(context.Background())
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	c.client.Transport = throttle.New(nil, 1000, 1, cerebrasClassify)

	result, err := c.Classify(context.Background(), "Subject", "sender@example.com", "Hello", []Category{{Name: "next_steps", Description: "Follow-up"}, {Name: "other", Description: "Everything else"}})
	if err != nil {
		t.Fatalf("Classify() returned error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("request count = %d, want 2", calls)
	}
	if result.Category != "next_steps" || result.Confidence != 0.75 {
		t.Fatalf("result = %+v, want category next_steps confidence 0.75", result)
	}
}

func TestClassifier_ClassifySendsDescriptionsAndAcceptsOther(t *testing.T) {
	categories := []Category{
		{Name: "interview", Description: "Invitation to schedule an interview."},
		{Name: "other", Description: "Anything unrelated to a job search."},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat struct {
				JSONSchema struct {
					Schema struct {
						Properties struct {
							Category struct {
								Enum []string `json:"enum"`
							} `json:"category"`
						} `json:"properties"`
					} `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error: %v", err)
		}
		if len(payload.Messages) != 1 {
			t.Fatalf("messages = %d, want 1", len(payload.Messages))
		}
		prompt := payload.Messages[0].Content
		for _, category := range categories {
			if want := "- " + category.Name + ": " + category.Description; !strings.Contains(prompt, want) {
				t.Fatalf("prompt missing %q:\n%s", want, prompt)
			}
		}
		enum := payload.ResponseFormat.JSONSchema.Schema.Properties.Category.Enum
		if strings.Join(enum, ",") != "interview,other" {
			t.Fatalf("category enum = %v, want [interview other]", enum)
		}

		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"category\":\"other\",\"confidence\":0.93}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	oldURL := cerebrasAPIURL
	cerebrasAPIURL = server.URL + "/v1/chat/completions"
	defer func() { cerebrasAPIURL = oldURL }()

	t.Setenv("CEREBRAS_API_KEY", "test-key")
	c, err := New(context.Background())
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	result, err := c.Classify(context.Background(), "Weekly newsletter", "news@example.com", "Top stories this week", categories)
	if err != nil {
		t.Fatalf("Classify() returned error: %v", err)
	}
	if result.Category != "other" || result.Confidence != 0.93 {
		t.Fatalf("result = %+v, want category other confidence 0.93", result)
	}
}

func TestCategoriesFromModels_PreservesNamesAndDescriptions(t *testing.T) {
	rows := []models.Category{
		{ID: 1, Name: "confirmation", Description: "Application received."},
		{ID: 6, Name: "other", Description: "Everything else."},
	}

	got := CategoriesFromModels(rows)
	want := []Category{
		{Name: "confirmation", Description: "Application received."},
		{Name: "other", Description: "Everything else."},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("category[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestBuildPrompt_DirectsWeakMatchesToOther(t *testing.T) {
	prompt := buildPrompt("Subject", "sender@example.com", "Body", []Category{
		{Name: "interview", Description: "Interview scheduling."},
		{Name: "other", Description: "Anything else."},
	})

	if !strings.Contains(prompt, `choose "other"`) {
		t.Fatalf("prompt does not direct weak matches to other:\n%s", prompt)
	}
	if strings.Contains(prompt, "closest category anyway") {
		t.Fatalf("prompt still forces closest category:\n%s", prompt)
	}
}

func TestClassifier_ClassifyRequiresOtherCategory(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))
	defer server.Close()

	oldURL := cerebrasAPIURL
	cerebrasAPIURL = server.URL + "/v1/chat/completions"
	defer func() { cerebrasAPIURL = oldURL }()

	t.Setenv("CEREBRAS_API_KEY", "test-key")
	c, err := New(context.Background())
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	_, err = c.Classify(context.Background(), "Subject", "sender@example.com", "Hello", []Category{{Name: "next_steps", Description: "Follow-up"}})
	if !errors.Is(err, ErrMissingOtherCategory) {
		t.Fatalf("Classify() error = %v, want ErrMissingOtherCategory", err)
	}
	if calls != 0 {
		t.Fatalf("request count = %d, want 0", calls)
	}
}

func TestBuildPrompt_FormatsCategoryDescriptions(t *testing.T) {
	prompt := buildPrompt("Subject", "sender@example.com", "Body", []Category{
		{Name: "interview", Description: "  Interview scheduling.  "},
		{Name: "next_steps", Description: ""},
	})

	if !strings.Contains(prompt, "- interview: Interview scheduling.\n") {
		t.Fatalf("prompt missing trimmed description line:\n%s", prompt)
	}
	if !strings.Contains(prompt, "- next_steps\n") || strings.Contains(prompt, "- next_steps:") {
		t.Fatalf("category without description should be listed without a separator:\n%s", prompt)
	}
}

func TestCerebrasClassify(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		head     string
		body     string
		wantWait time.Duration
		wantOK   bool
	}{
		{name: "429 with Retry-After", status: http.StatusTooManyRequests, head: "5", body: `rate limit exceeded`, wantWait: 5 * time.Second, wantOK: true},
		{name: "429 daily quota", status: http.StatusTooManyRequests, head: "", body: `daily quota exceeded for this account`, wantWait: 0, wantOK: false},
		{name: "503 with Retry-After", status: http.StatusServiceUnavailable, head: "2", body: `temporarily unavailable`, wantWait: 2 * time.Second, wantOK: true},
		{name: "503 without Retry-After", status: http.StatusServiceUnavailable, head: "", body: `temporarily unavailable`, wantWait: 0, wantOK: true},
		{name: "200", status: http.StatusOK, head: "", body: `ok`, wantWait: 0, wantOK: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			head := make(http.Header)
			head.Set("Retry-After", tc.head)
			resp := &http.Response{StatusCode: tc.status, Header: head}
			wait, ok := cerebrasClassify(resp, []byte(tc.body))
			if ok != tc.wantOK {
				t.Fatalf("cerebrasClassify ok = %v, want %v", ok, tc.wantOK)
			}
			if wait != tc.wantWait {
				t.Fatalf("cerebrasClassify wait = %v, want %v", wait, tc.wantWait)
			}
		})
	}
}
