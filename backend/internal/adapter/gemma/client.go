// Package gemma adapts the local Voxis Source-Available Gemma runtime to application ports.
package gemma

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/voxis/backend/internal/config"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/prompts/oss"
)

const (
	defaultMaxCompletionTokens = 4096
	chatTemplateTokenReserve   = 256

	// EndpointLabel is recorded as a summary's endpoint location. The real
	// runtime URL is internal topology and must not reach API or MCP clients.
	EndpointLabel = "local-gemma"
	// modelConcurrency bounds in-flight requests to the local runtime, which
	// serves one model on shared hardware.
	modelConcurrency = 2
	// interactiveModelWait is how long an interactive request waits for a free
	// slot before it fails with domain.ErrModelBusy.
	interactiveModelWait = 30 * time.Second

	// Benchmark26BModel is an approved larger instruction-tuned comparison
	// candidate. It is not a production Voxis-OSS model choice.
	Benchmark26BModel = "google/gemma-4-26B-A4B-it"
	// Benchmark26BQATModel is the official QAT GGUF comparison candidate.
	Benchmark26BQATModel = "google/gemma-4-26B-A4B-it-qat-q4_0-gguf"
)

// Client calls the operator-configured local OpenAI-compatible Gemma endpoint.
type Client struct {
	cfg        config.GemmaConfig
	endpoint   string
	http       *http.Client
	promptRepo port.LLMPromptRepository
	// slots is the process-wide model semaphore; main builds one Client.
	slots           chan struct{}
	interactiveWait time.Duration
}

// Option configures the Gemma adapter.
type Option func(*Client)

// WithPromptRepository enables public summary presentation overrides.
func WithPromptRepository(repo port.LLMPromptRepository) Option {
	return func(client *Client) { client.promptRepo = repo }
}

// NewClient creates an adapter for the configured, allowlisted Gemma model.
func NewClient(cfg config.GemmaConfig, opts ...Option) (*Client, error) {
	return newClient(cfg, http.DefaultClient, opts...)
}

func newClient(cfg config.GemmaConfig, httpClient *http.Client, opts ...Option) (*Client, error) {
	return newClientWithValidator(cfg, httpClient, validateConfig, opts...)
}

// NewBenchmarkClient creates a separate, allowlisted client for approved model
// comparisons. Production composition must use NewClient.
func NewBenchmarkClient(cfg config.GemmaConfig, opts ...Option) (*Client, error) {
	return newClientWithValidator(cfg, http.DefaultClient, validateBenchmarkConfig, opts...)
}

func newClientWithValidator(
	cfg config.GemmaConfig,
	httpClient *http.Client,
	validate func(config.GemmaConfig) error,
	opts ...Option,
) (*Client, error) {
	if err := validate(cfg); err != nil {
		return nil, err
	}
	if httpClient == nil {
		return nil, fmt.Errorf("gemma: HTTP client is required: %w", domain.ErrInvalidInput)
	}
	httpClient = rejectRedirects(httpClient)
	client := &Client{
		cfg:      cfg,
		endpoint: strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions",
		http:     httpClient,
		slots:    make(chan struct{}, modelConcurrency),

		interactiveWait: interactiveModelWait,
	}
	for _, opt := range opts {
		opt(client)
	}
	return client, nil
}

func rejectRedirects(source *http.Client) *http.Client {
	cloned := *source
	cloned.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &cloned
}

func validateConfig(cfg config.GemmaConfig) error {
	if cfg.Model != config.OSSGemmaModel && cfg.Model != config.OSSGemmaQATModel {
		return fmt.Errorf("gemma: unsupported model %q: %w", cfg.Model, domain.ErrInvalidInput)
	}
	return validateRuntimeConfig(cfg)
}

func validateBenchmarkConfig(cfg config.GemmaConfig) error {
	switch cfg.Model {
	case config.OSSGemmaModel, config.OSSGemmaQATModel, Benchmark26BModel, Benchmark26BQATModel:
		return validateRuntimeConfig(cfg)
	default:
		return fmt.Errorf("gemma: unsupported benchmark model %q: %w", cfg.Model, domain.ErrInvalidInput)
	}
}

func validateRuntimeConfig(cfg config.GemmaConfig) error {
	if err := validateRuntimeEndpoint(cfg.BaseURL, cfg.AllowHTTPInternal); err != nil {
		return err
	}
	if err := validateRuntimeMetadata(cfg); err != nil {
		return err
	}
	return validateRuntimeBounds(cfg)
}

func validateRuntimeEndpoint(rawURL string, allowHTTPInternal bool) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return fmt.Errorf("gemma: invalid base URL: %w", domain.ErrInvalidInput)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("gemma: base URL must not include credentials, query, or fragment: %w", domain.ErrInvalidInput)
	}
	if parsed.Scheme == "http" && !allowHTTPInternal {
		return fmt.Errorf("gemma: HTTP endpoint requires explicit internal allowance: %w", domain.ErrInvalidInput)
	}
	return nil
}

func validateRuntimeMetadata(cfg config.GemmaConfig) error {
	if strings.TrimSpace(cfg.ModelRevision) == "" || strings.TrimSpace(cfg.RuntimeRevision) == "" || strings.TrimSpace(cfg.Quantization) == "" {
		return fmt.Errorf("gemma: model, runtime, and quantization metadata are required: %w", domain.ErrInvalidInput)
	}
	return nil
}

func validateRuntimeBounds(cfg config.GemmaConfig) error {
	if cfg.MaxRequestBytes <= 0 || cfg.MaxResponseBytes <= 0 || cfg.MaxContextTokens <= chatTemplateTokenReserve || cfg.RequestTimeout <= 0 || cfg.MaxAttempts <= 0 {
		return fmt.Errorf("gemma: invalid bounded request configuration: %w", domain.ErrInvalidInput)
	}
	return nil
}

// GenerateSummary implements port.SummaryProvider with structured output only.
func (c *Client) GenerateSummary(ctx context.Context, req port.SummaryRequest) (*port.SummaryResult, error) {
	if req.ForceLegacy {
		return nil, fmt.Errorf("gemma: unstructured summary fallback is disabled in Voxis Source-Available: %w", domain.ErrGenerationUnsupported)
	}
	parts, promptRevision, err := c.summaryParts(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gemma: build summary prompt: %w", err)
	}
	completion, err := c.complete(ctx, parts, false)
	if err != nil {
		return nil, fmt.Errorf("gemma: generate summary: %w", err)
	}
	return c.summaryResult(completion, req.SourceVersion, req.SourceHash, promptRevision), nil
}

// ExtractSummaryAnchors implements port.AnchoredSummaryProvider.
func (c *Client) ExtractSummaryAnchors(ctx context.Context, req port.SummaryExtractionRequest) (*port.SummaryExtractionResult, error) {
	completion, err := c.complete(ctx, oss.Extraction(req), false)
	if err != nil {
		return nil, fmt.Errorf("gemma: extract summary anchors: %w", err)
	}
	var extraction extractionPayload
	if err := decodeStrictJSON(completion.content, &extraction); err != nil {
		return nil, invalidModelPayload("extraction")
	}
	if strings.TrimSpace(extraction.MatrixLanguage) == "" {
		return nil, fmt.Errorf("gemma: extraction has no matrix language: %w", domain.ErrGenerationUnsupported)
	}
	return &port.SummaryExtractionResult{
		ExtractionJSON:   string(completion.content),
		PromptTokens:     completion.promptTokens,
		CompletionTokens: completion.completionTokens,
		UsageAvailable:   completion.usageAvailable,
	}, nil
}

// GenerateSummaryFromAnchors implements port.AnchoredSummaryProvider.
func (c *Client) GenerateSummaryFromAnchors(ctx context.Context, req port.AnchoredSummaryRequest) (*port.SummaryResult, error) {
	parts, promptRevision, err := c.anchoredSummaryParts(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gemma: build anchored summary prompt: %w", err)
	}
	completion, err := c.complete(ctx, parts, false)
	if err != nil {
		return nil, fmt.Errorf("gemma: generate anchored summary: %w", err)
	}
	return c.summaryResult(completion, req.SourceVersion, req.SourceHash, promptRevision), nil
}

// SuggestSpeakers implements port.SpeakerSuggestionProvider.
func (c *Client) SuggestSpeakers(ctx context.Context, req port.SpeakerSuggestionRequest) (*port.SpeakerSuggestionResult, error) {
	if strings.TrimSpace(req.Transcript) == "" {
		return &port.SpeakerSuggestionResult{Suggestions: map[string]domain.SpeakerSuggestion{}}, nil
	}
	completion, err := c.complete(ctx, oss.Speakers(req), true)
	if err != nil {
		return nil, fmt.Errorf("gemma: suggest speakers: %w", err)
	}
	var payload speakerPayload
	if err := decodeStrictJSON(completion.content, &payload); err != nil {
		return nil, invalidModelPayload("speaker suggestions")
	}
	suggestions := make(map[string]domain.SpeakerSuggestion, len(payload.Speakers))
	for _, speaker := range payload.Speakers {
		suggestions[fmt.Sprintf("%d", speaker.Index)] = domain.SpeakerSuggestion{
			Name:       speaker.Name,
			Evidence:   speaker.Evidence,
			Confidence: speaker.Confidence,
		}
	}
	return &port.SpeakerSuggestionResult{
		Suggestions:      suggestions,
		PromptTokens:     completion.promptTokens,
		CompletionTokens: completion.completionTokens,
	}, nil
}

// AnswerTranscriptQuestion implements port.TranscriptAnswerer.
func (c *Client) AnswerTranscriptQuestion(ctx context.Context, question string, evidence []port.TranscriptEvidence) (*port.TranscriptAnswer, error) {
	if len(evidence) == 0 {
		return &port.TranscriptAnswer{}, nil
	}
	completion, err := c.complete(ctx, oss.TranscriptAnswer(question, evidence), true)
	if err != nil {
		return nil, fmt.Errorf("gemma: answer transcript question: %w", err)
	}
	var payload answerPayload
	if err := decodeStrictJSON(completion.content, &payload); err != nil {
		return nil, invalidModelPayload("transcript answer")
	}
	return &port.TranscriptAnswer{Answer: strings.TrimSpace(payload.Answer), CitationIDs: payload.CitationIDs}, nil
}

// DefaultSummaryPromptConfigs exposes the public prompt controls for the
// configured, fixed model. Callers must not offer other model choices.
func (c *Client) DefaultSummaryPromptConfigs() []port.LLMPromptConfig {
	return oss.DefaultSummaryPromptConfigs(c.cfg.Model)
}

func (c *Client) summaryParts(ctx context.Context, req port.SummaryRequest) (oss.Parts, string, error) {
	presentation, err := c.summaryPresentation(ctx, req.SummaryType)
	if err != nil {
		return oss.Parts{}, "", err
	}
	parts, err := oss.SummaryWithPresentation(req, presentation)
	if err != nil {
		return oss.Parts{}, "", err
	}
	return parts, oss.PromptRevision(parts), nil
}

func (c *Client) anchoredSummaryParts(ctx context.Context, req port.AnchoredSummaryRequest) (oss.Parts, string, error) {
	presentation, err := c.summaryPresentation(ctx, req.SummaryType)
	if err != nil {
		return oss.Parts{}, "", err
	}
	parts, err := oss.AnchoredSummaryWithPresentation(req, presentation)
	if err != nil {
		return oss.Parts{}, "", err
	}
	return parts, oss.PromptRevision(parts), nil
}

func (c *Client) summaryPresentation(ctx context.Context, summaryType string) (oss.SummaryPresentation, error) {
	defaults := c.DefaultSummaryPromptConfigs()
	for _, cfg := range defaults {
		if cfg.SummaryType != summaryType {
			continue
		}
		return c.overridePresentation(ctx, cfg)
	}
	return oss.SummaryPresentation{}, fmt.Errorf("gemma: unsupported summary prompt: %w", domain.ErrGenerationUnsupported)
}

func (c *Client) overridePresentation(ctx context.Context, defaultConfig port.LLMPromptConfig) (oss.SummaryPresentation, error) {
	presentation := oss.SummaryPresentation{
		SystemInstruction: defaultConfig.SystemInstruction,
		UserPrompt:        defaultConfig.UserPrompt,
	}
	if c.promptRepo == nil {
		return presentation, nil
	}
	override, err := c.promptRepo.GetOverride(ctx, defaultConfig.Key)
	if errors.Is(err, domain.ErrNotFound) {
		return presentation, nil
	}
	if err != nil {
		return oss.SummaryPresentation{}, fmt.Errorf("gemma: prompt override unavailable: %w", domain.ErrGenerationUnsupported)
	}
	if strings.TrimSpace(override.Model) != c.cfg.Model {
		return oss.SummaryPresentation{}, fmt.Errorf("gemma: prompt override selected a different model: %w", domain.ErrGenerationUnsupported)
	}
	if strings.TrimSpace(override.SystemInstruction) == "" || strings.TrimSpace(override.UserPrompt) == "" {
		return oss.SummaryPresentation{}, fmt.Errorf("gemma: prompt override is incomplete: %w", domain.ErrGenerationUnsupported)
	}
	presentation.SystemInstruction = override.SystemInstruction
	presentation.UserPrompt = override.UserPrompt
	return presentation, nil
}

func (c *Client) summaryResult(completion completionResult, sourceVersion, sourceHash, promptRevision string) *port.SummaryResult {
	content := append([]byte(nil), completion.content...)
	return &port.SummaryResult{
		Content:                 string(content),
		StructuredContent:       content,
		StructuredSchemaVersion: domain.StructuredSummarySchemaVersion,
		PromptVersion:           promptRevision,
		Model:                   c.cfg.Model,
		ModelRevision:           c.cfg.ModelRevision,
		RuntimeRevision:         c.cfg.RuntimeRevision,
		Quantization:            c.cfg.Quantization,
		EndpointLocation:        EndpointLabel,
		SourceVersion:           sourceVersion,
		SourceHash:              sourceHash,
		WordCount:               len(strings.Fields(string(content))),
		PromptTokens:            completion.promptTokens,
		CompletionTokens:        completion.completionTokens,
		UsageAvailable:          completion.usageAvailable,
	}
}

// complete sends one request, with bounded retries, while holding a model
// slot. Interactive callers (questions, briefs, speaker suggestions) wait at
// most interactiveModelWait; background summary jobs wait for their context.
func (c *Client) complete(ctx context.Context, parts oss.Parts, interactive bool) (completionResult, error) {
	body, err := c.requestBody(parts)
	if err != nil {
		return completionResult{}, err
	}
	release, err := c.acquireSlot(ctx, interactive)
	if err != nil {
		return completionResult{}, err
	}
	defer release()
	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		result, retry, err := c.call(ctx, body)
		if err == nil || !retry || attempt == c.cfg.MaxAttempts {
			return result, err
		}
		if err := waitRetry(ctx, attempt); err != nil {
			return completionResult{}, err
		}
	}
	return completionResult{}, fmt.Errorf("gemma: exhausted attempts: %w", domain.ErrInternal)
}

// acquireSlot takes one of the modelConcurrency slots and returns its release.
func (c *Client) acquireSlot(ctx context.Context, interactive bool) (func(), error) {
	release := func() { <-c.slots }
	if !interactive {
		select {
		case c.slots <- struct{}{}:
			return release, nil
		case <-ctx.Done():
			return nil, fmt.Errorf("gemma: wait for model slot: %w", ctx.Err())
		}
	}
	timer := time.NewTimer(c.interactiveWait)
	defer timer.Stop()
	select {
	case c.slots <- struct{}{}:
		return release, nil
	case <-timer.C:
		return nil, fmt.Errorf("gemma: local model busy: %w", domain.ErrModelBusy)
	case <-ctx.Done():
		return nil, fmt.Errorf("gemma: wait for model slot: %w", ctx.Err())
	}
}

func (c *Client) requestBody(parts oss.Parts) ([]byte, error) {
	maxCompletionTokens := c.maxCompletionTokens()
	if len(parts.System)+len(parts.User)+chatTemplateTokenReserve+maxCompletionTokens > c.cfg.MaxContextTokens {
		return nil, fmt.Errorf("gemma: prompt exceeds configured context limit without truncation: %w", domain.ErrGenerationUnsupported)
	}
	request := chatRequest{
		Model: c.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: parts.System},
			{Role: "user", Content: parts.User},
		},
		Temperature:         0,
		MaxCompletionTokens: maxCompletionTokens,
		ResponseFormat: responseFormat{
			Type:       "json_schema",
			JSONSchema: responseJSONSchema{Name: parts.SchemaName, Strict: true, Schema: parts.Schema},
		},
		ChatTemplateKwargs: chatTemplateKwargs{EnableThinking: false},
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("gemma: marshal request: %w", err)
	}
	if int64(len(body)) > c.cfg.MaxRequestBytes {
		return nil, fmt.Errorf("gemma: request exceeds configured size limit: %w", domain.ErrGenerationUnsupported)
	}
	return body, nil
}

func (c *Client) maxCompletionTokens() int {
	limit := c.cfg.MaxContextTokens / 4
	if limit < defaultMaxCompletionTokens {
		return limit
	}
	return defaultMaxCompletionTokens
}

func (c *Client) call(ctx context.Context, body []byte) (completionResult, bool, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return completionResult{}, false, fmt.Errorf("gemma: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return completionResult{}, false, ctx.Err()
		}
		return completionResult{}, true, fmt.Errorf("gemma: call local runtime: %w", err)
	}
	data, err := readBounded(resp.Body, c.cfg.MaxResponseBytes)
	if err != nil {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return completionResult{}, false, errors.Join(err, fmt.Errorf("gemma: close response: %w", closeErr))
		}
		return completionResult{}, false, err
	}
	if err := resp.Body.Close(); err != nil {
		return completionResult{}, false, fmt.Errorf("gemma: close response: %w", err)
	}
	return parseResponse(resp.StatusCode, data, c.cfg.Model)
}

func readBounded(body io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("gemma: read response: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("gemma: response exceeds configured size limit: %w", domain.ErrGenerationUnsupported)
	}
	return data, nil
}

func parseResponse(status int, data []byte, expectedModel string) (completionResult, bool, error) {
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return completionResult{}, transientStatus(status), statusError(status)
	}
	var response chatResponse
	if err := decodeJSON(data, &response); err != nil {
		return completionResult{}, false, fmt.Errorf("gemma: malformed runtime response: %w", domain.ErrGenerationUnsupported)
	}
	if response.Model != expectedModel {
		return completionResult{}, false, fmt.Errorf("gemma: runtime served an unexpected model: %w", domain.ErrGenerationUnsupported)
	}
	if len(response.Choices) != 1 {
		return completionResult{}, false, fmt.Errorf("gemma: expected one completion choice: %w", domain.ErrGenerationUnsupported)
	}
	choice := response.Choices[0]
	if err := validateFinishReason(choice.FinishReason); err != nil {
		return completionResult{}, false, err
	}
	content := []byte(strings.TrimSpace(choice.Message.Content))
	if len(content) == 0 || hasReasoningMarker(string(content)) {
		return completionResult{}, false, fmt.Errorf("gemma: unusable final content: %w", domain.ErrGenerationUnsupported)
	}
	// OpenAI-compatible runtimes may omit usage. The port has integer fields, so
	// zero means unavailable here; token counts are never estimated from text.
	result := completionResult{content: content}
	if response.Usage != nil {
		result.promptTokens = response.Usage.PromptTokens
		result.completionTokens = response.Usage.CompletionTokens
		result.usageAvailable = true
	}
	return result, false, nil
}

func transientStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func statusError(status int) error {
	if status == http.StatusBadRequest || status == http.StatusUnprocessableEntity {
		return fmt.Errorf("gemma: runtime rejected structured request (HTTP %d): %w", status, domain.ErrStructuredRequestRejected)
	}
	return fmt.Errorf("gemma: runtime returned HTTP %d: %w", status, domain.ErrGenerationUnsupported)
}

func validateFinishReason(reason string) error {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "stop":
		return nil
	case "length":
		return fmt.Errorf("gemma: completion reached output limit: %w", domain.ErrGenerationIncomplete)
	case "content_filter":
		return fmt.Errorf("gemma: completion was blocked: %w", domain.ErrContentBlocked)
	default:
		return fmt.Errorf("gemma: unsupported completion finish reason: %w", domain.ErrGenerationUnsupported)
	}
}

func waitRetry(ctx context.Context, attempt int) error {
	delay := time.Duration(attempt) * 50 * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func hasReasoningMarker(content string) bool {
	value := strings.ToLower(content)
	for _, marker := range []string{"<think", "<|channel>thought", "<|channel|>thought", "<|channel>analysis", "<|channel|>analysis"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func invalidModelPayload(kind string) error {
	return fmt.Errorf("gemma: %s did not match the required JSON shape: %w", kind, domain.ErrGenerationUnsupported)
}

func decodeJSON(data []byte, target interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON values")
	}
	return nil
}

func decodeStrictJSON(data []byte, target interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON values")
	}
	return nil
}

type chatRequest struct {
	Model               string             `json:"model"`
	Messages            []chatMessage      `json:"messages"`
	Temperature         int                `json:"temperature"`
	MaxCompletionTokens int                `json:"max_completion_tokens"`
	ResponseFormat      responseFormat     `json:"response_format"`
	ChatTemplateKwargs  chatTemplateKwargs `json:"chat_template_kwargs"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type       string             `json:"type"`
	JSONSchema responseJSONSchema `json:"json_schema"`
}

type responseJSONSchema struct {
	Name   string          `json:"name"`
	Strict bool            `json:"strict"`
	Schema json.RawMessage `json:"schema"`
}

type chatTemplateKwargs struct {
	EnableThinking bool `json:"enable_thinking"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
	Usage   *chatUsage   `json:"usage"`
	Model   string       `json:"model"`
}

type chatChoice struct {
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type completionResult struct {
	content          []byte
	promptTokens     int
	completionTokens int
	usageAvailable   bool
}

type extractionPayload struct {
	MatrixLanguage string             `json:"matrix_language"`
	Anchors        []extractionAnchor `json:"anchors"`
	Uncertainties  []string           `json:"uncertainties"`
}

type extractionAnchor struct {
	Fact        string   `json:"fact"`
	CitationIDs []string `json:"citation_ids"`
}

type speakerPayload struct {
	Speakers []speakerEntry `json:"speakers"`
}

type speakerEntry struct {
	Index      int    `json:"index"`
	Name       string `json:"name"`
	Confidence string `json:"confidence"`
	Evidence   string `json:"evidence"`
}

type answerPayload struct {
	Answer      string   `json:"answer"`
	CitationIDs []string `json:"citation_ids"`
}

var _ port.SummaryProvider = (*Client)(nil)
var _ port.AnchoredSummaryProvider = (*Client)(nil)
var _ port.SpeakerSuggestionProvider = (*Client)(nil)
var _ port.TranscriptAnswerer = (*Client)(nil)
