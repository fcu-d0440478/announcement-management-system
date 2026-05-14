package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS announcement_reads (
	announcement_id BIGINT NOT NULL REFERENCES announcements(id) ON DELETE CASCADE,
	user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	read_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (announcement_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_announcements_status_publish ON announcements(status, publish_at);
CREATE INDEX IF NOT EXISTS idx_announcements_category ON announcements(category_id);
CREATE INDEX IF NOT EXISTS idx_announcements_search ON announcements USING gin (to_tsvector('simple', title || ' ' || content));
`
	if _, err := s.DB.ExecContext(ctx, schema); err != nil {
		return err
	}
	return s.seed(ctx)
}

func (s *Store) seed(ctx context.Context) error {
	categories := []models.Category{
		{Name: "HR", Description: "People, policy and organization notices"},
		{Name: "IT", Description: "System maintenance and security notices"},
		{Name: "Event", Description: "Company events and training"},
	}
	for _, category := range categories {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO categories (name, description) VALUES ($1, $2) ON CONFLICT (name) DO NOTHING`, category.Name, category.Description); err != nil {
			return err
		}
	}

	users := []struct {
		username string
		password string
		role     string
		name     string
	}{
		{"admin", "admin123", "admin", "System Admin"},
		{"editor", "editor123", "editor", "Announcement Editor"},
		{"user", "user123", "user", "Employee User"},
	}
	for _, user := range users {
		hash, err := auth.HashPassword(user.password)
		if err != nil {
			return err
		}
		_, err = s.DB.ExecContext(ctx, `
INSERT INTO users (username, password_hash, role, name)
VALUES ($1, $2, $3, $4)
ON CONFLICT (username) DO NOTHING`, user.username, hash, user.role, user.name)
		if err != nil {
			return err
		}
	}

	var adminID int64
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM users WHERE username = 'admin'`).Scan(&adminID); err != nil {
		return err
	}

	demos := []struct {
		title        string
		content      string
		categoryName string
		status       string
		publishAt    string
		expiresAt    *string
	}{
		{
			title:        "Announcement system launched",
			content:      "The internal announcement system is online. Please sign in, review notices, and mark them as read.",
			categoryName: "IT",
			status:       "published",
			publishAt:    "now()",
		},
		{
			title:        "Quarterly maintenance window",
			content:      "Core services will be maintained this Friday evening. Please save work before the maintenance window starts.",
			categoryName: "IT",
			status:       "scheduled",
			publishAt:    "now() + interval '2 hours'",
		},
		{
			title:        "Employee handbook draft",
			content:      "The HR team is reviewing the next handbook update. Editors can revise this draft before publishing.",
			categoryName: "HR",
			status:       "draft",
			publishAt:    "NULL",
		},
		{
			title:        "Annual health check registration",
			content:      "Employees can register for the annual health check from Monday. Please complete the form before the deadline.",
			categoryName: "HR",
			status:       "published",
			publishAt:    "now() - interval '1 day'",
		},
		{
			title:        "Town hall replay archived",
			content:      "The previous town hall notice has been archived. Managers may still review it from the admin dashboard.",
			categoryName: "Event",
			status:       "archived",
			publishAt:    "now() - interval '14 days'",
		},
		{
			title:        "Product training workshop",
			content:      "A product training workshop will be held next week. The session is open to all departments.",
			categoryName: "Event",
			status:       "published",
			publishAt:    "now() - interval '3 hours'",
		},
	}

	for _, demo := range demos {
		var categoryID int64
		if err := s.DB.QueryRowContext(ctx, `SELECT id FROM categories WHERE name = $1`, demo.categoryName).Scan(&categoryID); err != nil {
			return err
		}
		query := fmt.Sprintf(`
INSERT INTO announcements (title, content, category_id, status, publish_at, expires_at, created_by)
SELECT $1, $2, $3, $4, %s, NULL, $5
WHERE NOT EXISTS (SELECT 1 FROM announcements WHERE title = $1)`, demo.publishAt)
		if _, err := s.DB.ExecContext(ctx, query, demo.title, demo.content, categoryID, demo.status, adminID); err != nil {
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
	UnreadOnly bool
}

func (s *Store) Announcements(ctx context.Context, filter AnnouncementFilter) ([]models.Announcement, error) {
	args := []interface{}{filter.UserID}
	query := `
SELECT a.id, a.title, a.content, a.category_id, c.name, a.status, a.publish_at, a.expires_at,
       a.created_by, u.name, a.created_at, a.updated_at,
       EXISTS (SELECT 1 FROM announcement_reads ar WHERE ar.announcement_id = a.id AND ar.user_id = $1) AS is_read,
       (SELECT COUNT(*) FROM announcement_reads ar WHERE ar.announcement_id = a.id) AS read_count
FROM announcements a
JOIN categories c ON c.id = a.category_id
JOIN users u ON u.id = a.created_by
WHERE 1 = 1`
	if !auth.CanManage(filter.Role) {
		query += ` AND a.status = 'published' AND (a.publish_at IS NULL OR a.publish_at <= now()) AND (a.expires_at IS NULL OR a.expires_at > now())`
	}
	if filter.Query != "" {
		args = append(args, "%"+filter.Query+"%")
		query += fmt.Sprintf(` AND (a.title ILIKE $%d OR a.content ILIKE $%d)`, len(args), len(args))
	}
	if filter.CategoryID > 0 {
		args = append(args, filter.CategoryID)
		query += fmt.Sprintf(` AND a.category_id = $%d`, len(args))
	}
	if filter.Status != "" && auth.CanManage(filter.Role) {
		args = append(args, filter.Status)
		query += fmt.Sprintf(` AND a.status = $%d`, len(args))
	}
	if filter.UnreadOnly {
		query += ` AND NOT EXISTS (SELECT 1 FROM announcement_reads ar WHERE ar.announcement_id = a.id AND ar.user_id = $1)`
	}
	query += ` ORDER BY COALESCE(a.publish_at, a.created_at) DESC, a.id DESC`
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var announcements []models.Announcement
	for rows.Next() {
		var announcement models.Announcement
		if err := rows.Scan(&announcement.ID, &announcement.Title, &announcement.Content, &announcement.CategoryID, &announcement.Category,
			&announcement.Status, &announcement.PublishAt, &announcement.ExpiresAt, &announcement.CreatedBy, &announcement.AuthorName,
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
	err := s.DB.QueryRowContext(ctx, `
INSERT INTO announcements (title, content, category_id, status, publish_at, expires_at, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, created_at, updated_at`, announcement.Title, announcement.Content, announcement.CategoryID, announcement.Status, announcement.PublishAt, announcement.ExpiresAt, announcement.CreatedBy).
		Scan(&announcement.ID, &announcement.CreatedAt, &announcement.UpdatedAt)
	return announcement, err
}

func (s *Store) UpdateAnnouncement(ctx context.Context, id int64, announcement models.Announcement) error {
	result, err := s.DB.ExecContext(ctx, `
UPDATE announcements
SET title = $1, content = $2, category_id = $3, status = $4, publish_at = $5, expires_at = $6, updated_at = now()
WHERE id = $7`, announcement.Title, announcement.Content, announcement.CategoryID, announcement.Status, announcement.PublishAt, announcement.ExpiresAt, id)
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
	return nil
}

func (s *Store) DeleteAnnouncement(ctx context.Context, id int64) error {
	result, err := s.DB.ExecContext(ctx, `DELETE FROM announcements WHERE id = $1`, id)
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
	return nil
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
	result, err := s.DB.ExecContext(ctx, `UPDATE announcements SET status = 'published', updated_at = now() WHERE status = 'scheduled' AND publish_at <= now()`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
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
