package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

const (
	maxPromptTextLen = 20_000
	maxModelLen      = 128
)

var modelNamePattern = regexp.MustCompile(`^[A-Za-z0-9._:/-]+$`)

// LLMPromptUpdate is an admin request to replace one task, presentation, and
// model override. The grounding and injection invariant remains code-owned.
type LLMPromptUpdate struct {
	SystemInstruction string
	UserPrompt        string
	Model             string
}

// LLMPromptService manages global LLM prompt/model overrides.
type LLMPromptService struct {
	repo          port.LLMPromptRepository
	defaults      func() []port.LLMPromptConfig
	allowedModels map[string]bool
}

// NewLLMPromptService creates an LLMPromptService.
func NewLLMPromptService(
	repo port.LLMPromptRepository,
	defaults func() []port.LLMPromptConfig,
) *LLMPromptService {
	return NewLLMPromptServiceWithAllowedModels(repo, defaults, nil)
}

// NewLLMPromptServiceWithAllowedModels creates an LLMPromptService with a
// server-side model allowlist for admin overrides.
func NewLLMPromptServiceWithAllowedModels(
	repo port.LLMPromptRepository,
	defaults func() []port.LLMPromptConfig,
	allowedModels []string,
) *LLMPromptService {
	if repo == nil {
		panic("service: prompt repo must not be nil")
	}
	if defaults == nil {
		panic("service: prompt defaults must not be nil")
	}
	return &LLMPromptService{
		repo:          repo,
		defaults:      defaults,
		allowedModels: buildAllowedModelSet(defaults, allowedModels),
	}
}

// List returns effective prompt configs by merging persisted overrides into defaults.
func (s *LLMPromptService) List(ctx context.Context) ([]port.LLMPromptConfig, error) {
	defaults := s.defaults()
	overrides, err := s.repo.ListOverrides(ctx)
	if err != nil {
		return nil, fmt.Errorf("list prompt overrides: %w", err)
	}

	byKey := make(map[string]port.LLMPromptOverride, len(overrides))
	for _, override := range overrides {
		byKey[override.Key] = override
	}

	out := make([]port.LLMPromptConfig, 0, len(defaults))
	for _, cfg := range defaults {
		cfg.PromptVersion = defaultPromptVersion(cfg.PromptVersion)
		if override, ok := byKey[cfg.Key]; ok {
			s.applyPromptOverride(&cfg, override)
		}
		out = append(out, cfg)
	}
	return out, nil
}

// Get returns one effective prompt config.
func (s *LLMPromptService) Get(ctx context.Context, key string) (*port.LLMPromptConfig, error) {
	cfg, ok := s.defaultByKey(key)
	if !ok {
		return nil, fmt.Errorf("unknown prompt key %q: %w", key, domain.ErrInvalidInput)
	}
	override, err := s.repo.GetOverride(ctx, key)
	if err == nil {
		s.applyPromptOverride(&cfg, *override)
		return &cfg, nil
	}
	// err is non-nil here; a missing override simply means the default applies.
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("get prompt override: %w", err)
	}
	return &cfg, nil
}

// Update validates and stores an override for one prompt key.
func (s *LLMPromptService) Update(
	ctx context.Context,
	key string,
	update LLMPromptUpdate,
	updatedBy string,
) (*port.LLMPromptConfig, error) {
	cfg, ok := s.defaultByKey(key)
	if !ok {
		return nil, fmt.Errorf("unknown prompt key %q: %w", key, domain.ErrInvalidInput)
	}
	override, err := s.validatePromptUpdate(key, update, updatedBy)
	if err != nil {
		return nil, err
	}
	stored, err := s.repo.UpsertOverride(ctx, override)
	if err != nil {
		return nil, fmt.Errorf("store prompt override: %w", err)
	}
	s.applyPromptOverride(&cfg, *stored)
	return &cfg, nil
}

// Reset deletes an override and returns the default config.
func (s *LLMPromptService) Reset(ctx context.Context, key string) (*port.LLMPromptConfig, error) {
	cfg, ok := s.defaultByKey(key)
	if !ok {
		return nil, fmt.Errorf("unknown prompt key %q: %w", key, domain.ErrInvalidInput)
	}
	if err := s.repo.DeleteOverride(ctx, key); err != nil {
		return nil, fmt.Errorf("delete prompt override: %w", err)
	}
	return &cfg, nil
}

func (s *LLMPromptService) defaultByKey(key string) (port.LLMPromptConfig, bool) {
	for _, cfg := range s.defaults() {
		if cfg.Key == key {
			cfg.PromptVersion = defaultPromptVersion(cfg.PromptVersion)
			return cfg, true
		}
	}
	return port.LLMPromptConfig{}, false
}

func defaultPromptVersion(version string) string {
	if strings.TrimSpace(version) != "" {
		return version
	}
	return port.SummaryPromptVersion
}

func (s *LLMPromptService) validatePromptUpdate(
	key string,
	update LLMPromptUpdate,
	updatedBy string,
) (port.LLMPromptOverride, error) {
	system := strings.TrimSpace(update.SystemInstruction)
	userPrompt := strings.TrimSpace(update.UserPrompt)
	model := strings.TrimSpace(update.Model)
	if system == "" || userPrompt == "" || model == "" {
		return port.LLMPromptOverride{}, fmt.Errorf("prompt fields cannot be blank: %w", domain.ErrInvalidInput)
	}
	if len(system) > maxPromptTextLen || len(userPrompt) > maxPromptTextLen {
		return port.LLMPromptOverride{}, fmt.Errorf("prompt text exceeds %d characters: %w", maxPromptTextLen, domain.ErrInvalidInput)
	}
	if len(model) > maxModelLen || !modelNamePattern.MatchString(model) {
		return port.LLMPromptOverride{}, fmt.Errorf("invalid model name: %w", domain.ErrInvalidInput)
	}
	if !s.allowedModels[model] {
		return port.LLMPromptOverride{}, fmt.Errorf("model is not allowed for prompt overrides: %w", domain.ErrInvalidInput)
	}
	return port.LLMPromptOverride{
		Key:               key,
		SystemInstruction: system,
		UserPrompt:        userPrompt,
		Model:             model,
		UpdatedBy:         strings.TrimSpace(updatedBy),
	}, nil
}

func buildAllowedModelSet(defaults func() []port.LLMPromptConfig, configured []string) map[string]bool {
	allowed := make(map[string]bool, len(configured)+len(defaults()))
	for _, cfg := range defaults() {
		model := strings.TrimSpace(cfg.Model)
		if model != "" {
			allowed[model] = true
		}
	}
	for _, model := range configured {
		model = strings.TrimSpace(model)
		if model != "" {
			allowed[model] = true
		}
	}
	return allowed
}

func (s *LLMPromptService) applyPromptOverride(cfg *port.LLMPromptConfig, override port.LLMPromptOverride) {
	// Vertex composes the code-owned source and injection invariant before this
	// admin presentation addendum and task prompt.
	cfg.SystemInstruction = override.SystemInstruction
	cfg.UserPrompt = override.UserPrompt
	if s.allowedModels[override.Model] {
		cfg.Model = override.Model
	}
	cfg.HasOverride = true
	cfg.UpdatedAt = &override.UpdatedAt
}
