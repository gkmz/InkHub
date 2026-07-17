package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	domainjob "github.com/gkmz/InkHub/internal/domain/job"
)

// JobRepository 持久化后台任务并提供原子领取能力。
type JobRepository struct {
	db *sql.DB
}

// NewJobRepository 创建后台任务 Repository。
func NewJobRepository(db *sql.DB) *JobRepository {
	return &JobRepository{db: db}
}

// Enqueue 创建任务；活动去重键已存在时返回原任务。
func (r *JobRepository) Enqueue(ctx context.Context, value domainjob.Job) (domainjob.Job, bool, error) {
	if value.ID == "" || value.WorkspaceID == "" || value.Kind == "" {
		return domainjob.Job{}, false, fmt.Errorf("任务身份字段不完整")
	}
	if value.PayloadJSON == "" {
		value.PayloadJSON = "{}"
	}
	if value.ResultJSON == "" {
		value.ResultJSON = "{}"
	}
	if value.AvailableAt.IsZero() {
		value.AvailableAt = time.Now().UTC()
	}
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `INSERT INTO jobs(
id,workspace_id,kind,dedupe_key,state,progress,payload_json,result_json,attempts,available_at,created_at,updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING`,
		value.ID, value.WorkspaceID, value.Kind, nullableString(value.DedupeKey), domainjob.StateQueued,
		0, value.PayloadJSON, value.ResultJSON, 0, formatJobTime(value.AvailableAt), formatJobTime(now), formatJobTime(now))
	if err != nil {
		return domainjob.Job{}, false, fmt.Errorf("任务入队: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domainjob.Job{}, false, fmt.Errorf("检查任务入队结果: %w", err)
	}
	if affected == 1 {
		created, err := r.FindByID(ctx, value.ID)
		return created, true, err
	}
	if value.DedupeKey == "" {
		return domainjob.Job{}, false, fmt.Errorf("任务 ID 已存在: %s", value.ID)
	}
	existing, err := r.findActiveByDedupe(ctx, value.WorkspaceID, value.DedupeKey)
	if err != nil {
		return domainjob.Job{}, false, err
	}
	return existing, false, nil
}

// ClaimNext 原子领取当前可运行的最早任务。
func (r *JobRepository) ClaimNext(ctx context.Context, now time.Time) (domainjob.Job, bool, error) {
	row := r.db.QueryRowContext(ctx, `UPDATE jobs SET
state='running',attempts=attempts+1,started_at=?,updated_at=?
WHERE id=(SELECT id FROM jobs WHERE state='queued' AND available_at<=? ORDER BY available_at,created_at LIMIT 1)
  AND state='queued'
RETURNING id,workspace_id,kind,dedupe_key,state,progress,payload_json,result_json,error_code,error_message,
attempts,available_at,started_at,finished_at,created_at,updated_at`,
		formatJobTime(now), formatJobTime(now), formatJobTime(now))
	value, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domainjob.Job{}, false, nil
	}
	if err != nil {
		return domainjob.Job{}, false, fmt.Errorf("领取任务: %w", err)
	}
	return value, true, nil
}

// FindByID 按任务 ID 查询持久化快照。
func (r *JobRepository) FindByID(ctx context.Context, id string) (domainjob.Job, error) {
	value, err := scanJob(r.db.QueryRowContext(ctx, `SELECT
id,workspace_id,kind,dedupe_key,state,progress,payload_json,result_json,error_code,error_message,
attempts,available_at,started_at,finished_at,created_at,updated_at FROM jobs WHERE id=?`, id))
	if err != nil {
		return domainjob.Job{}, fmt.Errorf("查询任务: %w", err)
	}
	return value, nil
}

// FindLatestTargetJob 按工作区、文章、Provider、内容版本和类型查询最新任务。
func (r *JobRepository) FindLatestTargetJob(ctx context.Context, workspaceID, articleID, providerID, contentHash, kind string) (domainjob.Job, bool, error) {
	value, err := scanJob(r.db.QueryRowContext(ctx, `SELECT
id,workspace_id,kind,dedupe_key,state,progress,payload_json,result_json,error_code,error_message,
attempts,available_at,started_at,finished_at,created_at,updated_at FROM jobs
WHERE workspace_id=? AND kind=?
  AND json_extract(payload_json,'$.article_id')=?
  AND json_extract(payload_json,'$.provider_instance_id')=?
  AND json_extract(payload_json,'$.content_hash')=?
ORDER BY created_at DESC,id DESC LIMIT 1`, workspaceID, kind, articleID, providerID, contentHash))
	if errors.Is(err, sql.ErrNoRows) {
		return domainjob.Job{}, false, nil
	}
	if err != nil {
		return domainjob.Job{}, false, fmt.Errorf("查询目标最新任务: %w", err)
	}
	return value, true, nil
}

// UpdateProgress 更新运行中任务的有限进度值。
func (r *JobRepository) UpdateProgress(ctx context.Context, id string, progress int, now time.Time) error {
	if progress < 0 || progress > 99 {
		return fmt.Errorf("运行中任务进度必须在 0 到 99 之间")
	}
	return r.updateRunning(ctx, id, `progress=?,updated_at=?`, progress, formatJobTime(now))
}

// Succeed 将运行中任务标记成功并保存结果。
func (r *JobRepository) Succeed(ctx context.Context, id, resultJSON string, now time.Time) error {
	if resultJSON == "" {
		resultJSON = "{}"
	}
	return r.updateRunning(ctx, id, `state='succeeded',progress=100,result_json=?,error_code=NULL,error_message=NULL,finished_at=?,updated_at=?`,
		resultJSON, formatJobTime(now), formatJobTime(now))
}

// Retry 将运行中任务按退避时间重新排队。
func (r *JobRepository) Retry(ctx context.Context, id string, availableAt time.Time, code, message string, now time.Time) error {
	return r.updateRunning(ctx, id, `state='queued',error_code=?,error_message=?,available_at=?,started_at=NULL,updated_at=?`,
		nullableString(code), nullableString(message), formatJobTime(availableAt), formatJobTime(now))
}

// Fail 将运行中任务标记为不可自动恢复的失败。
func (r *JobRepository) Fail(ctx context.Context, id, code, message string, now time.Time) error {
	return r.updateRunning(ctx, id, `state='failed',error_code=?,error_message=?,finished_at=?,updated_at=?`,
		nullableString(code), nullableString(message), formatJobTime(now), formatJobTime(now))
}

// Cancel 将排队或运行中的任务标记为已取消。
func (r *JobRepository) Cancel(ctx context.Context, id string, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE jobs SET state='cancelled',error_code='job.cancelled',
error_message='任务已取消',finished_at=?,updated_at=? WHERE id=? AND state IN ('queued','running')`,
		formatJobTime(now), formatJobTime(now), id)
	if err != nil {
		return fmt.Errorf("取消任务: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("检查任务取消结果: %w", err)
	}
	if affected == 1 {
		return nil
	}
	value, findErr := r.FindByID(ctx, id)
	if findErr == nil && value.State == domainjob.StateCancelled {
		return nil
	}
	return fmt.Errorf("任务不可取消: %s", id)
}

// ListRunningBefore 返回启动恢复阶段判定为超时的运行中任务。
func (r *JobRepository) ListRunningBefore(ctx context.Context, before time.Time) ([]domainjob.Job, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
id,workspace_id,kind,dedupe_key,state,progress,payload_json,result_json,error_code,error_message,
attempts,available_at,started_at,finished_at,created_at,updated_at FROM jobs
WHERE state='running' AND started_at<? ORDER BY started_at,id`, formatJobTime(before))
	if err != nil {
		return nil, fmt.Errorf("查询待恢复任务: %w", err)
	}
	defer rows.Close()
	var result []domainjob.Job
	for rows.Next() {
		value, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("解析待恢复任务: %w", err)
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历待恢复任务: %w", err)
	}
	return result, nil
}

// RequeueRecovered 将可安全恢复的遗留任务重新排队。
func (r *JobRepository) RequeueRecovered(ctx context.Context, id string, availableAt, now time.Time) error {
	return r.updateRunning(ctx, id, `state='queued',error_code='job.recovered',error_message='应用重启后重新排队',
available_at=?,started_at=NULL,updated_at=?`, formatJobTime(availableAt), formatJobTime(now))
}

// RequeueFailed 按任务身份原子重排一个最终失败的确定性任务。
func (r *JobRepository) RequeueFailed(ctx context.Context, id, workspaceID, kind string, now time.Time) (domainjob.Job, error) {
	value, err := scanJob(r.db.QueryRowContext(ctx, `UPDATE jobs SET
state='queued',progress=0,error_code=NULL,error_message=NULL,available_at=?,started_at=NULL,finished_at=NULL,updated_at=?
WHERE id=? AND workspace_id=? AND kind=? AND state='failed'
RETURNING id,workspace_id,kind,dedupe_key,state,progress,payload_json,result_json,error_code,error_message,
attempts,available_at,started_at,finished_at,created_at,updated_at`,
		formatJobTime(now), formatJobTime(now), id, workspaceID, kind))
	if errors.Is(err, sql.ErrNoRows) {
		return domainjob.Job{}, fmt.Errorf("失败任务身份或状态不匹配: %s", id)
	}
	if err != nil {
		return domainjob.Job{}, fmt.Errorf("重排失败任务: %w", err)
	}
	return value, nil
}

// FailRecovered 将无法安全恢复的遗留任务标记失败。
func (r *JobRepository) FailRecovered(ctx context.Context, id string, now time.Time) error {
	return r.updateRunning(ctx, id, `state='failed',error_code='job.recovery_unsafe',
error_message='任务中断后无法安全自动重试',finished_at=?,updated_at=?`, formatJobTime(now), formatJobTime(now))
}

func (r *JobRepository) updateRunning(ctx context.Context, id, assignments string, args ...any) error {
	query := `UPDATE jobs SET ` + assignments + ` WHERE id=? AND state='running'`
	args = append(args, id)
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("更新运行中任务: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("检查任务更新结果: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("任务不处于 running 状态: %s", id)
	}
	return nil
}

func (r *JobRepository) findActiveByDedupe(ctx context.Context, workspaceID, dedupeKey string) (domainjob.Job, error) {
	value, err := scanJob(r.db.QueryRowContext(ctx, `SELECT
id,workspace_id,kind,dedupe_key,state,progress,payload_json,result_json,error_code,error_message,
attempts,available_at,started_at,finished_at,created_at,updated_at FROM jobs
WHERE workspace_id=? AND dedupe_key=? AND state IN ('queued','running')`, workspaceID, dedupeKey))
	if err != nil {
		return domainjob.Job{}, fmt.Errorf("查询重复任务: %w", err)
	}
	return value, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanJob(row rowScanner) (domainjob.Job, error) {
	var value domainjob.Job
	var dedupeKey, errorCode, errorMessage sql.NullString
	var availableAt, createdAt, updatedAt string
	var startedAt, finishedAt sql.NullString
	err := row.Scan(
		&value.ID, &value.WorkspaceID, &value.Kind, &dedupeKey, &value.State, &value.Progress,
		&value.PayloadJSON, &value.ResultJSON, &errorCode, &errorMessage, &value.Attempts,
		&availableAt, &startedAt, &finishedAt, &createdAt, &updatedAt,
	)
	if err != nil {
		return domainjob.Job{}, err
	}
	value.DedupeKey = dedupeKey.String
	value.ErrorCode = errorCode.String
	value.ErrorMessage = errorMessage.String
	var parseErr error
	if value.AvailableAt, parseErr = time.Parse(time.RFC3339Nano, availableAt); parseErr != nil {
		return domainjob.Job{}, fmt.Errorf("解析任务可用时间: %w", parseErr)
	}
	if value.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, createdAt); parseErr != nil {
		return domainjob.Job{}, fmt.Errorf("解析任务创建时间: %w", parseErr)
	}
	if value.UpdatedAt, parseErr = time.Parse(time.RFC3339Nano, updatedAt); parseErr != nil {
		return domainjob.Job{}, fmt.Errorf("解析任务更新时间: %w", parseErr)
	}
	if value.StartedAt, parseErr = parseJobTime(startedAt); parseErr != nil {
		return domainjob.Job{}, fmt.Errorf("解析任务开始时间: %w", parseErr)
	}
	if value.FinishedAt, parseErr = parseJobTime(finishedAt); parseErr != nil {
		return domainjob.Job{}, fmt.Errorf("解析任务完成时间: %w", parseErr)
	}
	return value, nil
}

func formatJobTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseJobTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
