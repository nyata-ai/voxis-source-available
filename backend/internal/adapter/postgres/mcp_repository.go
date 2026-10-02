package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/voxis/backend/internal/adapter/postgres/sqlcdb"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// Compile-time check that MCPRepository implements port.MCPRepository.
var _ port.MCPRepository = (*MCPRepository)(nil)

// MCPRepository provides MCP-specific queries for cross-entity search
// and filtered listing.
type MCPRepository struct {
	pool    *pgxpool.Pool
	queries *sqlcdb.Queries
}

// NewMCPRepository creates a new MCP repository.
func NewMCPRepository(pool *pgxpool.Pool) *MCPRepository {
	return &MCPRepository{
		pool:    pool,
		queries: sqlcdb.New(pool),
	}
}

// SearchAllEntities performs a unified ILIKE search across media and
// standard media. Results are ordered by creation time descending.
func (r *MCPRepository) SearchAllEntities(ctx context.Context, orgID, search string, limit, offset int) ([]port.SearchAllResult, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	rows, err := r.queries.SearchAllEntities(ctx, sqlcdb.SearchAllEntitiesParams{
		OrganizationID: orgUUID,
		Search:         search,
		LimitVal:       int32(limit),  //nolint:gosec // bounded by handler (1..100)
		OffsetVal:      int32(offset), //nolint:gosec // bounded by handler (>= 0)
	})
	if err != nil {
		return nil, fmt.Errorf("search all entities: %w", err)
	}

	results := make([]port.SearchAllResult, len(rows))
	for i, row := range rows {
		results[i] = port.SearchAllResult{
			ID:                    row.ID,
			EntityType:            row.EntityType,
			Title:                 row.Title,
			Description:           row.Description,
			Filename:              row.Filename,
			Status:                row.Status,
			CreatedAt:             row.CreatedAt.Format("2006-01-02T15:04:05Z"),
			LatestTranscriptionID: interfaceToString(row.LatestTranscriptionID),
		}
	}
	return results, nil
}

// CountSearchAllEntities counts standard media matching SearchAllEntities.
func (r *MCPRepository) CountSearchAllEntities(ctx context.Context, orgID, search string) (int64, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return 0, domain.ErrNotFound
	}
	total, err := r.queries.CountSearchAllEntities(ctx, sqlcdb.CountSearchAllEntitiesParams{
		OrganizationID: orgUUID,
		Search:         search,
	})
	if err != nil {
		return 0, fmt.Errorf("count search all entities: %w", err)
	}
	return total, nil
}

// CreateCollection creates an MCP transcript collection.
func (r *MCPRepository) CreateCollection(
	ctx context.Context,
	orgID, name, description, createdBy string,
) (*port.MCPCollection, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	row, err := r.queries.CreateMCPCollection(ctx, sqlcdb.CreateMCPCollectionParams{
		OrganizationID: orgUUID,
		Name:           name,
		Description:    description,
		CreatedBy:      createdBy,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, domain.ErrConflict
		}
		return nil, fmt.Errorf("create mcp collection: %w", err)
	}
	collection := collectionFromCreateRow(row)
	return &collection, nil
}

// ListCollections lists MCP collections in an organization.
func (r *MCPRepository) ListCollections(ctx context.Context, orgID string, limit, offset int) ([]port.MCPCollection, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return nil, domain.ErrNotFound
	}
	rows, err := r.queries.ListMCPCollections(ctx, sqlcdb.ListMCPCollectionsParams{
		OrganizationID: orgUUID,
		LimitVal:       int32(limit),  //nolint:gosec // bounded by service/handler
		OffsetVal:      int32(offset), //nolint:gosec // bounded by service/handler
	})
	if err != nil {
		return nil, fmt.Errorf("list mcp collections: %w", err)
	}
	items := make([]port.MCPCollection, len(rows))
	for i, row := range rows {
		items[i] = collectionFromListRow(row)
	}
	return items, nil
}

// GetCollection retrieves one MCP collection by ID with org isolation.
func (r *MCPRepository) GetCollection(ctx context.Context, orgID, collectionID string) (*port.MCPCollection, error) {
	orgUUID, collectionUUID, err := parseCollectionIDs(orgID, collectionID)
	if err != nil {
		return nil, err
	}
	row, err := r.queries.GetMCPCollection(ctx, sqlcdb.GetMCPCollectionParams{
		OrganizationID: orgUUID,
		CollectionID:   collectionUUID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get mcp collection: %w", err)
	}
	collection := collectionFromGetRow(row)
	return &collection, nil
}

// AddCollectionItem adds a transcription to a collection idempotently.
func (r *MCPRepository) AddCollectionItem(ctx context.Context, orgID, collectionID, transcriptionID string) error {
	orgUUID, collectionUUID, transUUID, err := parseCollectionItemIDs(orgID, collectionID, transcriptionID)
	if err != nil {
		return err
	}
	if err := r.queries.AddMCPCollectionItem(ctx, sqlcdb.AddMCPCollectionItemParams{
		OrganizationID:  orgUUID,
		CollectionID:    collectionUUID,
		TranscriptionID: transUUID,
	}); err != nil {
		return fmt.Errorf("add mcp collection item: %w", err)
	}
	return nil
}

// RemoveCollectionItem removes a transcription from a collection idempotently.
func (r *MCPRepository) RemoveCollectionItem(ctx context.Context, orgID, collectionID, transcriptionID string) error {
	orgUUID, collectionUUID, transUUID, err := parseCollectionItemIDs(orgID, collectionID, transcriptionID)
	if err != nil {
		return err
	}
	if err := r.queries.RemoveMCPCollectionItem(ctx, sqlcdb.RemoveMCPCollectionItemParams{
		OrganizationID:  orgUUID,
		CollectionID:    collectionUUID,
		TranscriptionID: transUUID,
	}); err != nil {
		return fmt.Errorf("remove mcp collection item: %w", err)
	}
	return nil
}

// ListCollectionItems lists collection items with org isolation.
func (r *MCPRepository) ListCollectionItems(
	ctx context.Context,
	orgID, collectionID string,
	limit, offset int,
) ([]port.MCPCollectionItem, error) {
	orgUUID, collectionUUID, err := parseCollectionIDs(orgID, collectionID)
	if err != nil {
		return nil, err
	}
	rows, err := r.queries.ListMCPCollectionItems(ctx, sqlcdb.ListMCPCollectionItemsParams{
		OrganizationID: orgUUID,
		CollectionID:   collectionUUID,
		LimitVal:       int32(limit),  //nolint:gosec // bounded by service/handler
		OffsetVal:      int32(offset), //nolint:gosec // bounded by service/handler
	})
	if err != nil {
		return nil, fmt.Errorf("list mcp collection items: %w", err)
	}
	items := make([]port.MCPCollectionItem, len(rows))
	for i, row := range rows {
		items[i] = port.MCPCollectionItem{
			CollectionID:    row.CollectionID,
			TranscriptionID: row.TranscriptionID,
			AddedAt:         row.AddedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return items, nil
}

// ListTranscriptionsWithoutSummaries returns completed transcriptions that
// lack a completed summary of the given type. An empty summaryType means
// "missing any completed summary of any type".
func (r *MCPRepository) ListTranscriptionsWithoutSummaries(ctx context.Context, orgID, summaryType string, limit, offset int) ([]port.TranscriptionWithoutSummary, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	rows, err := r.queries.ListTranscriptionsWithoutSummaries(ctx, sqlcdb.ListTranscriptionsWithoutSummariesParams{
		OrganizationID: orgUUID,
		SummaryType:    summaryType,
		LimitVal:       int32(limit),  //nolint:gosec // bounded by handler (1..100)
		OffsetVal:      int32(offset), //nolint:gosec // bounded by handler (>= 0)
	})
	if err != nil {
		return nil, fmt.Errorf("list transcriptions without summaries: %w", err)
	}

	results := make([]port.TranscriptionWithoutSummary, len(rows))
	for i, row := range rows {
		results[i] = port.TranscriptionWithoutSummary{
			ID:              row.ID,
			SourceType:      row.SourceType,
			Status:          row.Status,
			WordCount:       int(row.WordCount),
			SpeakerCount:    int(row.SpeakerCount),
			DurationSeconds: row.DurationSeconds,
			CreatedAt:       row.CreatedAt.Format("2006-01-02T15:04:05Z"),
			DisplayName:     row.DisplayName,
			Description:     row.Description,
		}
	}
	return results, nil
}

func parseCollectionIDs(orgID, collectionID string) (orgUUID, collectionUUID pgtype.UUID, err error) {
	orgUUID, err = parseUUID(orgID)
	if err != nil {
		return orgUUID, collectionUUID, domain.ErrNotFound
	}
	collectionUUID, err = parseUUID(collectionID)
	if err != nil {
		return orgUUID, collectionUUID, domain.ErrNotFound
	}
	return orgUUID, collectionUUID, nil
}

func parseCollectionItemIDs(orgID, collectionID, transcriptionID string) (
	orgUUID, collectionUUID, transUUID pgtype.UUID,
	err error,
) {
	orgUUID, collectionUUID, err = parseCollectionIDs(orgID, collectionID)
	if err != nil {
		return orgUUID, collectionUUID, transUUID, err
	}
	transUUID, err = parseUUID(transcriptionID)
	if err != nil {
		return orgUUID, collectionUUID, transUUID, domain.ErrNotFound
	}
	return orgUUID, collectionUUID, transUUID, nil
}

func collectionFromCreateRow(row sqlcdb.CreateMCPCollectionRow) port.MCPCollection {
	return port.MCPCollection{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		Name:           row.Name,
		Description:    row.Description,
		CreatedBy:      row.CreatedBy,
		CreatedAt:      row.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      row.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func collectionFromListRow(row sqlcdb.ListMCPCollectionsRow) port.MCPCollection {
	return port.MCPCollection{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		Name:           row.Name,
		Description:    row.Description,
		CreatedBy:      row.CreatedBy,
		CreatedAt:      row.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      row.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		ItemCount:      int(row.ItemCount),
	}
}

func collectionFromGetRow(row sqlcdb.GetMCPCollectionRow) port.MCPCollection {
	return port.MCPCollection{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		Name:           row.Name,
		Description:    row.Description,
		CreatedBy:      row.CreatedBy,
		CreatedAt:      row.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      row.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		ItemCount:      int(row.ItemCount),
	}
}

// interfaceToString safely converts an interface{} value to a string.
// Returns empty string for nil or non-string values.
func interfaceToString(v interface{}) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
