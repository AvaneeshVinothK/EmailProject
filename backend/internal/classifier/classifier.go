package classifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AvaneeshVinothK/EmailProject/internal/throttle"
)

const (
	defaultModel               = "gpt-oss-120b"
	defaultMaxCompletionTokens = 512
)

var modelVersion = defaultModel
var cerebrasAPIURL = "https://api.cerebras.ai/v1/chat/completions"

var ErrDailyQuotaExhausted = errors.New("classifier: daily quota exhausted")

// Category represents a job-search classification bucket and its explanatory description.
type Category struct {
	Name        string
	Description string
}

// Result is the structured classification decision returned by the model.
type Result struct {
	Category   string  `json:"category"`
	Confidence float64 `json:"confidence"`
}

// Classifier wraps the HTTP client used to classify emails.
type Classifier struct {
	client *http.Client
	apiKey string
	model  string
	url    string
}

// New constructs a Cerebras client using the configured API key.
func New(ctx context.Context) (*Classifier, error) {
	apiKey := strings.TrimSpace(os.Getenv("CEREBRAS_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("CEREBRAS_API_KEY is not set: %w", errors.New("missing Cerebras API key"))
	}

	model := strings.TrimSpace(os.Getenv("CEREBRAS_MODEL"))
	if model == "" {
		model = defaultModel
	}
	modelVersion = model

	return &Classifier{
		client: &http.Client{Transport: throttle.New(nil, 4.0/60.0, 1, cerebrasClassify), Timeout: 30 * time.Second},
		apiKey: apiKey,
		model:  model,
		url:    cerebrasAPIURL,
	}, nil
}

// ModelVersion returns the model used for email classification.
func ModelVersion() string {
	return modelVersion
}

// Classify assigns the most appropriate category to an email based on the provided categories.
func (c *Classifier) Classify(ctx context.Context, subject, sender, body string, categories []Category) (Result, error) {
	if c == nil || c.client == nil {
		return Result{}, fmt.Errorf("classifying email: %w", errors.New("classifier client is nil"))
	}
	if len(categories) == 0 {
		return Result{}, fmt.Errorf("classifying email: %w", errors.New("no categories provided"))
	}

	prompt := buildPrompt(subject, sender, body, categories)
	requestBody, err := json.Marshal(map[string]any{
		"model": c.model,
		"messages": []map[string]string{{
			"role":    "user",
			"content": prompt,
		}},
		"max_completion_tokens": defaultMaxCompletionTokens,
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "email_classification",
				"strict": true,
				"schema": buildSchema(categories),
			},
		},
	})
	if err != nil {
		return Result{}, fmt.Errorf("marshalling request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(requestBody))
	if err != nil {
		return Result{}, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("calling Cerebras chat completions: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, fmt.Errorf("reading Cerebras response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		bodyText := strings.TrimSpace(string(respBody))
		if resp.StatusCode == http.StatusTooManyRequests && isDailyQuotaExhausted(bodyText) {
			return Result{}, fmt.Errorf("Cerebras API returned status %d: %s: %w", resp.StatusCode, bodyText, ErrDailyQuotaExhausted)
		}
		return Result{}, fmt.Errorf("Cerebras API returned status %d: %s", resp.StatusCode, bodyText)
	}

	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return Result{}, fmt.Errorf("unmarshalling Cerebras response: %w", err)
	}
	if len(completion.Choices) == 0 {
		return Result{}, fmt.Errorf("parsing Cerebras response: empty choices (finish_reason=%q)", "")
	}

	content := strings.TrimSpace(completion.Choices[0].Message.Content)
	if content == "" {
		return Result{}, fmt.Errorf("parsing Cerebras response: empty content (finish_reason=%q)", completion.Choices[0].FinishReason)
	}

	var result Result
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return Result{}, fmt.Errorf("unmarshalling classification payload %q: %w", content, err)
	}

	allowed := make(map[string]bool, len(categories))
	for _, category := range categories {
		allowed[category.Name] = true
	}
	if !allowed[result.Category] {
		return Result{}, fmt.Errorf("invalid category %q: not in allowed categories", result.Category)
	}
	if result.Confidence < 0 || result.Confidence > 1 {
		return Result{}, fmt.Errorf("confidence %.3f out of range [0,1]", result.Confidence)
	}
	return result, nil
}

func cerebrasClassify(resp *http.Response, body []byte) (time.Duration, bool) {
	if resp == nil {
		return 0, false
	}
	if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusServiceUnavailable {
		return 0, false
	}

	bodyText := strings.TrimSpace(string(body))
	if resp.StatusCode == http.StatusTooManyRequests {
		if bodyText != "" {
			log.Printf("cerebras: 429 body: %s", bodyText)
		}
		if isDailyQuotaExhausted(bodyText) {
			return 0, false
		}
	}

	if v := strings.TrimSpace(resp.Header.Get("Retry-After")); v != "" {
		if seconds, err := strconv.Atoi(v); err == nil {
			return time.Duration(seconds) * time.Second, true
		}
		if when, err := http.ParseTime(v); err == nil {
			wait := time.Until(when)
			if wait < 0 {
				wait = 0
			}
			return wait, true
		}
	}
	return 0, true
}

func isDailyQuotaExhausted(body string) bool {
	text := strings.ToLower(body)
	return strings.Contains(text, "daily") || strings.Contains(text, "per day") || strings.Contains(text, "tpd") ||
		(strings.Contains(text, "quota") && strings.Contains(text, "exceeded"))
}

func buildPrompt(subject, sender, body string, categories []Category) string {
	var b strings.Builder
	b.WriteString("Classify this email for a job-search workflow. Return the single best category and confidence between 0 and 1.\n")
	b.WriteString("If no category is a strong fit, choose the closest category anyway with a lower confidence value.\n\n")
	b.WriteString("Available categories:\n")
	for _, category := range categories {
		b.WriteString(fmt.Sprintf("- %s: %s\n", category.Name, category.Description))
	}
	b.WriteString("\n")
	b.WriteString("Subject: ")
	b.WriteString(subject)
	b.WriteString("\n")
	b.WriteString("Sender: ")
	b.WriteString(sender)
	b.WriteString("\n\nEmail body:\n")
	b.WriteString(truncate(body, 3000))
	return b.String()
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

func buildSchema(categories []Category) map[string]any {
	categoryNames := make([]string, 0, len(categories))
	for _, category := range categories {
		categoryNames = append(categoryNames, category.Name)
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"category": map[string]any{
				"type": "string",
				"enum": categoryNames,
			},
			"confidence": map[string]any{
				"type": "number",
			},
		},
		"required":             []string{"category", "confidence"},
		"additionalProperties": false,
	}
}
