package aiassist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"menata.app/internal/config"
)

// geminiModel is the specific Gemini model this package calls. Named once here rather than
// scattered as a literal, so upgrading it is a one-line change.
const geminiModel = "gemini-2.5-flash"

const geminiEndpoint = "https://generativelanguage.googleapis.com/v1beta/models/" + geminiModel + ":generateContent"

// Turn is one message in the conversation, either side.
type Turn struct {
	Role string // "user" or "model"
	Text string
}

// Reply is one assistant turn, decoded from Gemini's own structured JSON output (see
// responseSchema in newRequestBody) -- never free-form prose the caller has to parse itself.
type Reply struct {
	// Message is what to show the user in the conversation log.
	Message string `json:"message"`
	// Change is set only once the assistant has enough information to propose a complete,
	// schema-shaped metadata change -- nil means "still gathering information", which is this
	// package's own mechanical half of "confirm until metadata is complete".
	Change *GeneratedChange `json:"change,omitempty"`
	// CapabilityGap is set when the request (or part of it) is not something this runtime can
	// express yet -- the model names the gap; internal/web's own handler is what records it
	// (aiassist itself never touches a database), per the kajian's own "the model names the gap,
	// the code decides nothing about what counts as one".
	CapabilityGap *CapabilityGap `json:"capability_gap,omitempty"`
}

// CapabilityGap is one thing the assistant told a user it cannot do yet.
type CapabilityGap struct {
	Requested string `json:"requested"`
	Note      string `json:"note"`
}

// Client is the one method this package needs from an AI backend -- narrow on purpose, so a test
// fake never has to implement more than a conversation actually uses.
type Client interface {
	Generate(ctx context.Context, systemPrompt string, turns []Turn) (Reply, error)
}

// NewClientFromConfig returns a real GeminiClient if cfg declares an API key, else an
// UnconfiguredClient -- mirrors internal/mail.NewMailerFromConfig's own posture, except this
// feature has no graceful degraded mode: an unconfigured assistant cannot converse at all, so its
// own entry point is hidden by internal/web rather than offered and then failing every message.
func NewClientFromConfig(cfg config.Config) Client {
	if cfg.GeminiAPIKey == "" {
		return UnconfiguredClient{}
	}
	return GeminiClient{APIKey: cfg.GeminiAPIKey, HTTPClient: &http.Client{Timeout: 60 * time.Second}}
}

// UnconfiguredClient is what NewClientFromConfig returns when no GEMINI_API_KEY is set. Unlike
// mail.LogMailer, it does not degrade to a working no-op -- there is no meaningful "log the
// conversation instead of having one" -- so it returns a clear, actionable error instead. In
// practice internal/web never lets a request reach this: the feature's own entry point is hidden
// from navigation whenever GeminiAPIKey is empty (same "hide, don't offer-then-403" convention
// ShowMembersAndGroups already uses).
type UnconfiguredClient struct{}

func (UnconfiguredClient) Generate(context.Context, string, []Turn) (Reply, error) {
	return Reply{}, fmt.Errorf("the AI assistant is not configured (GEMINI_API_KEY is unset)")
}

// GeminiClient calls Google's Generative Language API directly over HTTPS/JSON -- no SDK: the
// call is a plain REST request well within net/http/encoding/json, and this repo's own go.mod
// carries no HTTP client dependency today (confirmed by direct read); adding one for a single
// endpoint would be the first such dependency in the whole codebase.
type GeminiClient struct {
	APIKey     string
	HTTPClient *http.Client
}

func (c GeminiClient) Generate(ctx context.Context, systemPrompt string, turns []Turn) (Reply, error) {
	body, err := json.Marshal(newRequestBody(systemPrompt, turns))
	if err != nil {
		return Reply{}, fmt.Errorf("encode gemini request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, geminiEndpoint, bytes.NewReader(body))
	if err != nil {
		return Reply{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Reply{}, fmt.Errorf("call gemini: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Reply{}, fmt.Errorf("read gemini response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Reply{}, fmt.Errorf("gemini returned %s: %s", resp.Status, truncate(string(respBody), 500))
	}

	var parsed geminiResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Reply{}, fmt.Errorf("parse gemini response: %w", err)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return Reply{}, fmt.Errorf("gemini returned no candidates")
	}
	text := parsed.Candidates[0].Content.Parts[0].Text

	var reply Reply
	if err := json.Unmarshal([]byte(text), &reply); err != nil {
		return Reply{}, fmt.Errorf("gemini's own structured output did not match the expected shape: %w (raw: %s)", err, truncate(text, 500))
	}
	return reply, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// --- Gemini's own wire shapes --------------------------------------------------------------------

type geminiRequest struct {
	SystemInstruction geminiContent          `json:"system_instruction"`
	Contents          []geminiContent        `json:"contents"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerationConfig struct {
	ResponseMIMEType string       `json:"responseMimeType"`
	ResponseSchema   geminiSchema `json:"responseSchema"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
}

func newRequestBody(systemPrompt string, turns []Turn) geminiRequest {
	contents := make([]geminiContent, 0, len(turns))
	for _, t := range turns {
		contents = append(contents, geminiContent{Role: t.Role, Parts: []geminiPart{{Text: t.Text}}})
	}
	return geminiRequest{
		SystemInstruction: geminiContent{Parts: []geminiPart{{Text: systemPrompt}}},
		Contents:          contents,
		GenerationConfig: geminiGenerationConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema:   replySchema,
		},
	}
}
