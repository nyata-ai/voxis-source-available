package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// Compile-time check that LLMPromptRepository implements port.LLMPromptRepository.
var _ port.LLMPromptRepository = (*LLMPromptRepository)(nil)

// LLMPromptRepository implements port.LLMPromptRepository using PostgreSQL.
type LLMPromptRepository struct {
	pool *pgxpool.Pool
}

// NewLLMPromptRepository creates a PostgreSQL prompt repository.
func NewLLMPromptRepository(pool *pgxpool.Pool) *LLMPromptRepository {
	if pool == nil {
		panic("postgres: LLM prompt repository pool must not be nil")
	}
	return &LLMPromptRepository{pool: pool}
}

// ListOverrides returns all prompt overrides.
func (r *LLMPromptRepository) ListOverrides(ctx context.Context) ([]port.LLMPromptOverride, error) {
	rows, err := r.pool.Query(ctx, `
SELECT key, system_instruction, user_prompt, model, updated_at, updated_by
FROM llm_prompt_overrides
ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("list LLM prompt overrides: %w", err)
	}
	defer rows.Close()

	var overrides []port.LLMPromptOverride
	for rows.Next() {
		override, scanErr := scanLLMPromptOverride(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		overrides = append(overrides, override)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan LLM prompt overrides: %w", err)
	}
	return overrides, nil
}

// GetOverride returns one prompt override.
func (r *LLMPromptRepository) GetOverride(ctx context.Context, key string) (*port.LLMPromptOverride, error) {
	row := r.pool.QueryRow(ctx, `
SELECT key, system_instruction, user_prompt, model, updated_at, updated_by
FROM llm_prompt_overrides
WHERE key = $1`, key)
	override, err := scanLLMPromptOverride(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &override, nil
}

// UpsertOverride creates or replaces one prompt override.
func (r *LLMPromptRepository) UpsertOverride(
	ctx context.Context,
	override port.LLMPromptOverride,
) (*port.LLMPromptOverride, error) {
	row := r.pool.QueryRow(ctx, `
INSERT INTO llm_prompt_overrides (key, system_instruction, user_prompt, model, updated_by)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (key) DO UPDATE SET
  system_instruction = EXCLUDED.system_instruction,
  user_prompt = EXCLUDED.user_prompt,
  model = EXCLUDED.model,
  updated_by = EXCLUDED.updated_by,
  updated_at = NOW()
RETURNING key, system_instruction, user_prompt, model, updated_at, updated_by`,
		override.Key,
		override.SystemInstruction,
		override.UserPrompt,
		override.Model,
		override.UpdatedBy,
	)
	stored, err := scanLLMPromptOverride(row)
	if err != nil {
		return nil, fmt.Errorf("upsert LLM prompt override: %w", err)
	}
	return &stored, nil
}

// DeleteOverride removes one prompt override.
func (r *LLMPromptRepository) DeleteOverride(ctx context.Context, key string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM llm_prompt_overrides WHERE key = $1`, key)
	if err != nil {
		return fmt.Errorf("delete LLM prompt override: %w", err)
	}
	return nil
}

type llmPromptScanner interface {
	Scan(dest ...any) error
}

func scanLLMPromptOverride(row llmPromptScanner) (port.LLMPromptOverride, error) {
	var override port.LLMPromptOverride
	if err := row.Scan(
		&override.Key,
		&override.SystemInstruction,
		&override.UserPrompt,
		&override.Model,
		&override.UpdatedAt,
		&override.UpdatedBy,
	); err != nil {
		return port.LLMPromptOverride{}, fmt.Errorf("scan LLM prompt override: %w", err)
	}
	return override, nil
}
