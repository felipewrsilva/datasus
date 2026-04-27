package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"datasus/internal/domain"
)

type LogRepository struct {
	db *pgxpool.Pool
}

type ManualActionAudit struct {
	ID         int64           `json:"id"`
	Action     string          `json:"action"`
	Stage      *domain.StageName `json:"stage"`
	Actor      string          `json:"actor"`
	DetailsRaw []byte          `json:"details_json"`
	CreatedAt  string          `json:"created_at"`
}

func NewLogRepository(db *pgxpool.Pool) *LogRepository {
	return &LogRepository{db: db}
}

type LogEntry struct {
	ID          int64            `json:"id"`
	FileID      string           `json:"file_id"`
	Stage       domain.StageName `json:"stage"`
	EventType   string           `json:"event_type"`
	Message     string           `json:"message"`
	PayloadJSON []byte           `json:"payload_json"`
	CreatedAt   *string          `json:"created_at,omitempty"`
}

func (r *LogRepository) Insert(ctx context.Context, fileID string, stage domain.StageName, eventType, message string, payload any) error {
	var payloadJSON []byte
	if payload != nil {
		var err error
		payloadJSON, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal log payload: %w", err)
		}
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO download_logs (file_id, stage, event_type, message, payload_json)
		VALUES ($1, $2, $3, $4, $5)`,
		fileID, stage, eventType, message, payloadJSON)
	if err != nil {
		return fmt.Errorf("insert log: %w", err)
	}
	return nil
}

func (r *LogRepository) ListByFile(ctx context.Context, fileID string, limit int) ([]*LogEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, file_id, stage, event_type, message, payload_json, created_at::text
		FROM download_logs
		WHERE file_id=$1
		ORDER BY created_at DESC
		LIMIT $2`, fileID, limit)
	if err != nil {
		return nil, fmt.Errorf("list logs: %w", err)
	}
	defer rows.Close()

	var logs []*LogEntry
	for rows.Next() {
		e := &LogEntry{}
		if err := rows.Scan(&e.ID, &e.FileID, &e.Stage, &e.EventType, &e.Message, &e.PayloadJSON, &e.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, e)
	}
	return logs, rows.Err()
}

func (r *LogRepository) InsertManualAction(ctx context.Context, action string, stage *domain.StageName, actor string, details any) error {
	if strings.TrimSpace(actor) == "" {
		actor = "ui"
	}
	var payloadJSON []byte
	if details != nil {
		var err error
		payloadJSON, err = json.Marshal(details)
		if err != nil {
			return fmt.Errorf("marshal manual action payload: %w", err)
		}
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO manual_action_audit (action, stage, actor, details_json)
		VALUES ($1, $2, $3, $4)`,
		action, stage, actor, payloadJSON,
	)
	if err != nil {
		return fmt.Errorf("insert manual action audit: %w", err)
	}
	return nil
}

func (r *LogRepository) ListManualActions(ctx context.Context, limit int) ([]ManualActionAudit, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, action, stage, actor, details_json, created_at::text
		FROM manual_action_audit
		ORDER BY created_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list manual action audit: %w", err)
	}
	defer rows.Close()

	out := make([]ManualActionAudit, 0, limit)
	for rows.Next() {
		var item ManualActionAudit
		if err := rows.Scan(&item.ID, &item.Action, &item.Stage, &item.Actor, &item.DetailsRaw, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
