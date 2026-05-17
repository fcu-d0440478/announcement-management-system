package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"announcement-management-system/backend/internal/auth"
	"announcement-management-system/backend/internal/models"
	_ "github.com/lib/pq"
)

type Store struct {
	DB *sql.DB
}

func Connect(databaseURL string) (*Store, error) {
	database, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(15)
	database.SetMaxIdleConns(5)
	database.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for {
		if err := database.PingContext(ctx); err == nil {
			return &Store{DB: database}, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (s *Store) Migrate(ctx context.Context) error {
	schema := `
CREATE TABLE IF NOT EXISTS users (
	id BIGSERIAL PRIMARY KEY,
	username TEXT UNIQUE NOT NULL,
	password_hash TEXT NOT NULL,
	role TEXT NOT NULL CHECK (role IN ('admin', 'editor', 'user')),
	name TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS categories (
	id BIGSERIAL PRIMARY KEY,
	name TEXT UNIQUE NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS announcements (
	id BIGSERIAL PRIMARY KEY,
	title TEXT NOT NULL,
	content TEXT NOT NULL,
	category_id BIGINT NOT NULL REFERENCES categories(id),
	status TEXT NOT NULL CHECK (status IN ('draft', 'scheduled', 'published', 'archived')),
	publish_at TIMESTAMPTZ,
	expires_at TIMESTAMPTZ,
	created_by BIGINT NOT NULL REFERENCES users(id),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS announcement_reads (
	announcement_id BIGINT NOT NULL REFERENCES announcements(id) ON DELETE CASCADE,
	user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	read_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (announcement_id, user_id)
);

CREATE TABLE IF NOT EXISTS audit_logs (
	id BIGSERIAL PRIMARY KEY,
	user_id BIGINT NOT NULL REFERENCES users(id),
	action TEXT NOT NULL,
	target_type TEXT NOT NULL,
	target_id BIGINT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE announcements ADD COLUMN IF NOT EXISTS updated_by BIGINT REFERENCES users(id);
ALTER TABLE announcements ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
UPDATE announcements SET updated_by = created_by WHERE updated_by IS NULL;

CREATE INDEX IF NOT EXISTS idx_announcements_status_publish ON announcements(status, publish_at);
CREATE INDEX IF NOT EXISTS idx_announcements_category ON announcements(category_id);
CREATE INDEX IF NOT EXISTS idx_announcements_search ON announcements USING gin (to_tsvector('simple', title || ' ' || content));
CREATE INDEX IF NOT EXISTS idx_announcements_deleted_at ON announcements(deleted_at);
CREATE INDEX IF NOT EXISTS idx_audit_logs_target ON audit_logs(target_type, target_id, created_at DESC);
`
	if _, err := s.DB.ExecContext(ctx, schema); err != nil {
		return err
	}
	return s.seed(ctx)
}

func (s *Store) seed(ctx context.Context) error {
	categories := []struct {
		oldName     string
		name        string
		description string
	}{
		{oldName: "HR", name: "人事", description: "組織、人員與制度公告"},
		{oldName: "IT", name: "資訊", description: "系統維護、資安與 IT 通知"},
		{oldName: "Event", name: "活動", description: "公司活動與教育訓練"},
	}
	for _, category := range categories {
		if _, err := s.DB.ExecContext(ctx, `
UPDATE categories
SET name = $1, description = $2
WHERE name = $3 AND NOT EXISTS (SELECT 1 FROM categories WHERE name = $1)`, category.name, category.description, category.oldName); err != nil {
			return err
		}
		if _, err := s.DB.ExecContext(ctx, `
INSERT INTO categories (name, description)
VALUES ($1, $2)
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description`, category.name, category.description); err != nil {
			return err
		}
	}

	users := []struct {
		username string
		password string
		role     string
		name     string
	}{
		{"admin", "admin123", "admin", "系統管理員"},
		{"editor", "editor123", "editor", "公告編輯"},
		{"user", "user123", "user", "一般員工"},
	}
	for _, user := range users {
		hash, err := auth.HashPassword(user.password)
		if err != nil {
			return err
		}
		_, err = s.DB.ExecContext(ctx, `
INSERT INTO users (username, password_hash, role, name)
VALUES ($1, $2, $3, $4)
ON CONFLICT (username) DO UPDATE SET role = EXCLUDED.role, name = EXCLUDED.name`, user.username, hash, user.role, user.name)
		if err != nil {
			return err
		}
	}

	var adminID int64
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM users WHERE username = 'admin'`).Scan(&adminID); err != nil {
		return err
	}

	demos := []struct {
		oldTitle     string
		title        string
		content      string
		categoryName string
		status       string
		publishAt    string
		expiresAt    *string
	}{
		{
			oldTitle:     "Announcement system launched",
			title:        "公告系統正式上線",
			content:      "新版企業公告管理系統已上線，請同仁登入後確認公告並標記已讀。",
			categoryName: "資訊",
			status:       "published",
			publishAt:    "now()",
		},
		{
			oldTitle:     "Quarterly maintenance window",
			title:        "季度系統維護通知",
			content:      "核心服務將於本週五晚間進行維護，請同仁在維護開始前儲存手邊工作。",
			categoryName: "資訊",
			status:       "scheduled",
			publishAt:    "now() + interval '2 hours'",
		},
		{
			oldTitle:     "Employee handbook draft",
			title:        "員工手冊修訂草稿",
			content:      "人資團隊正在檢視新版員工手冊內容，編輯者可在發布前持續調整此草稿。",
			categoryName: "人事",
			status:       "draft",
			publishAt:    "NULL",
		},
		{
			oldTitle:     "Annual health check registration",
			title:        "年度健康檢查報名",
			content:      "年度健康檢查將於下週一開放報名，請同仁於期限前完成表單填寫。",
			categoryName: "人事",
			status:       "published",
			publishAt:    "now() - interval '1 day'",
		},
		{
			oldTitle:     "Town hall replay archived",
			title:        "全員大會回放公告已封存",
			content:      "上一場全員大會回放公告已封存，管理者仍可在後台檢視歷史公告。",
			categoryName: "活動",
			status:       "archived",
			publishAt:    "now() - interval '14 days'",
		},
		{
			oldTitle:     "Product training workshop",
			title:        "產品訓練工作坊",
			content:      "下週將舉辦產品訓練工作坊，歡迎各部門同仁報名參加。",
			categoryName: "活動",
			status:       "published",
			publishAt:    "now() - interval '3 hours'",
		},
	}

	for _, demo := range demos {
		var categoryID int64
		if err := s.DB.QueryRowContext(ctx, `SELECT id FROM categories WHERE name = $1`, demo.categoryName).Scan(&categoryID); err != nil {
			return err
		}
		if _, err := s.DB.ExecContext(ctx, `
UPDATE announcements
SET title = $1, content = $2, category_id = $3
WHERE title = $4`, demo.title, demo.content, categoryID, demo.oldTitle); err != nil {
			return err
		}
		query := fmt.Sprintf(`
INSERT INTO announcements (title, content, category_id, status, publish_at, expires_at, created_by, updated_by)
SELECT $1, $2, $3, $4, %s, NULL, $5, $5
WHERE NOT EXISTS (SELECT 1 FROM announcements WHERE title = $1 OR title = $6)`, demo.publishAt)
		if _, err := s.DB.ExecContext(ctx, query, demo.title, demo.content, categoryID, demo.status, adminID, demo.oldTitle); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) FindUserByUsername(ctx context.Context, username string) (models.User, error) {
	var user models.User
	err := s.DB.QueryRowContext(ctx, `SELECT id, username, password_hash, role, name FROM users WHERE username = $1`, username).
		Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.Name)
	return user, err
}

func (s *Store) Users(ctx context.Context) ([]models.User, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, username, role, name FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []models.User
	for rows.Next() {
		var user models.User
		if err := rows.Scan(&user.ID, &user.Username, &user.Role, &user.Name); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) Categories(ctx context.Context) ([]models.Category, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, description, created_at FROM categories ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var categories []models.Category
	for rows.Next() {
		var category models.Category
		if err := rows.Scan(&category.ID, &category.Name, &category.Description, &category.CreatedAt); err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	return categories, rows.Err()
}

func (s *Store) CreateCategory(ctx context.Context, category models.Category) (models.Category, error) {
	err := s.DB.QueryRowContext(ctx, `INSERT INTO categories (name, description) VALUES ($1, $2) RETURNING id, created_at`, category.Name, category.Description).
		Scan(&category.ID, &category.CreatedAt)
	return category, err
}

type AnnouncementFilter struct {
	UserID     int64
	Role       string
	Query      string
	CategoryID int64
	Status     string
	ReadState  string
}

func (s *Store) Announcements(ctx context.Context, filter AnnouncementFilter) ([]models.Announcement, error) {
	args := []interface{}{filter.UserID}
	query := `
SELECT a.id, a.title, a.content, a.category_id, c.name, a.status, a.publish_at, a.expires_at,
       a.created_by, u.name, COALESCE(editor.name, u.name) AS last_editor_name,
       last_read.name AS last_reader_name, last_read.read_at AS last_read_at,
       a.created_at, a.updated_at,
       EXISTS (SELECT 1 FROM announcement_reads ar WHERE ar.announcement_id = a.id AND ar.user_id = $1) AS is_read,
       (SELECT COUNT(*) FROM announcement_reads ar WHERE ar.announcement_id = a.id) AS read_count
FROM announcements a
JOIN categories c ON c.id = a.category_id
JOIN users u ON u.id = a.created_by
LEFT JOIN users editor ON editor.id = a.updated_by
LEFT JOIN LATERAL (
	SELECT ru.name, ar.read_at
	FROM announcement_reads ar
	JOIN users ru ON ru.id = ar.user_id
	WHERE ar.announcement_id = a.id
	ORDER BY ar.read_at DESC
	LIMIT 1
) last_read ON true
WHERE a.deleted_at IS NULL`
	if !auth.CanManage(filter.Role) {
		query += ` AND a.status = 'published' AND (a.publish_at IS NULL OR a.publish_at <= now()) AND (a.expires_at IS NULL OR a.expires_at > now())`
	}
	if filter.Query != "" {
		for _, term := range strings.Fields(filter.Query) {
			args = append(args, "%"+escapeLike(term)+"%")
			query += fmt.Sprintf(` AND (a.title ILIKE $%d ESCAPE '\' OR a.content ILIKE $%d ESCAPE '\')`, len(args), len(args))
		}
	}
	if filter.CategoryID > 0 {
		args = append(args, filter.CategoryID)
		query += fmt.Sprintf(` AND a.category_id = $%d`, len(args))
	}
	if filter.Status != "" && auth.CanManage(filter.Role) {
		args = append(args, filter.Status)
		query += fmt.Sprintf(` AND a.status = $%d`, len(args))
	}
	switch filter.ReadState {
	case "unread":
		query += ` AND NOT EXISTS (SELECT 1 FROM announcement_reads ar WHERE ar.announcement_id = a.id AND ar.user_id = $1)`
	case "read":
		query += ` AND EXISTS (SELECT 1 FROM announcement_reads ar WHERE ar.announcement_id = a.id AND ar.user_id = $1)`
	}
	query += ` ORDER BY COALESCE(a.publish_at, a.created_at) DESC, a.id DESC`
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	announcements := make([]models.Announcement, 0)
	for rows.Next() {
		var announcement models.Announcement
		if err := rows.Scan(&announcement.ID, &announcement.Title, &announcement.Content, &announcement.CategoryID, &announcement.Category,
			&announcement.Status, &announcement.PublishAt, &announcement.ExpiresAt, &announcement.CreatedBy, &announcement.AuthorName,
			&announcement.LastEditorName, &announcement.LastReaderName, &announcement.LastReadAt,
			&announcement.CreatedAt, &announcement.UpdatedAt, &announcement.IsRead, &announcement.ReadCount); err != nil {
			return nil, err
		}
		announcements = append(announcements, announcement)
	}
	return announcements, rows.Err()
}

func (s *Store) Announcement(ctx context.Context, id, userID int64, role string) (models.Announcement, error) {
	items, err := s.Announcements(ctx, AnnouncementFilter{UserID: userID, Role: role})
	if err != nil {
		return models.Announcement{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return models.Announcement{}, sql.ErrNoRows
}

func (s *Store) CreateAnnouncement(ctx context.Context, announcement models.Announcement) (models.Announcement, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return announcement, err
	}
	defer tx.Rollback()

	err = tx.QueryRowContext(ctx, `
INSERT INTO announcements (title, content, category_id, status, publish_at, expires_at, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
RETURNING id, created_at, updated_at`, announcement.Title, announcement.Content, announcement.CategoryID, announcement.Status, announcement.PublishAt, announcement.ExpiresAt, announcement.CreatedBy).
		Scan(&announcement.ID, &announcement.CreatedAt, &announcement.UpdatedAt)
	if err != nil {
		return announcement, err
	}
	if err := insertAuditLog(ctx, tx, announcement.CreatedBy, "create", "announcement", announcement.ID); err != nil {
		return announcement, err
	}
	if err := tx.Commit(); err != nil {
		return announcement, err
	}
	return announcement, err
}

func (s *Store) UpdateAnnouncement(ctx context.Context, id int64, announcement models.Announcement, updatedBy int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
UPDATE announcements
SET title = $1, content = $2, category_id = $3, status = $4, publish_at = $5, expires_at = $6, updated_by = $7, updated_at = now()
WHERE id = $8 AND deleted_at IS NULL`, announcement.Title, announcement.Content, announcement.CategoryID, announcement.Status, announcement.PublishAt, announcement.ExpiresAt, updatedBy, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	if err := insertAuditLog(ctx, tx, updatedBy, "update", "announcement", id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteAnnouncement(ctx context.Context, id int64, deletedBy int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `UPDATE announcements SET deleted_at = now(), updated_by = $2, updated_at = now() WHERE id = $1 AND deleted_at IS NULL`, id, deletedBy)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	if err := insertAuditLog(ctx, tx, deletedBy, "delete", "announcement", id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MarkRead(ctx context.Context, announcementID, userID int64) error {
	result, err := s.DB.ExecContext(ctx, `INSERT INTO announcement_reads (announcement_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, announcementID, userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return nil
	}
	return nil
}

func (s *Store) PromoteScheduled(ctx context.Context) (int64, error) {
	result, err := s.DB.ExecContext(ctx, `UPDATE announcements SET status = 'published', updated_at = now() WHERE status = 'scheduled' AND publish_at <= now() AND deleted_at IS NULL`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func insertAuditLog(ctx context.Context, tx *sql.Tx, userID int64, action string, targetType string, targetID int64) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO audit_logs (user_id, action, target_type, target_id)
VALUES ($1, $2, $3, $4)`, userID, action, targetType, targetID)
	return err
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}

func ValidateAnnouncement(announcement models.Announcement) error {
	if announcement.Title == "" || announcement.Content == "" || announcement.CategoryID == 0 {
		return errors.New("title, content and categoryId are required")
	}
	switch announcement.Status {
	case "draft", "scheduled", "published", "archived":
	default:
		return errors.New("invalid status")
	}
	if announcement.Status == "scheduled" && announcement.PublishAt == nil {
		return errors.New("publishAt is required for scheduled announcements")
	}
	return nil
}
