package models

import "time"

type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
	Name         string `json:"name"`
}

type Category struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Announcement struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Content     string     `json:"content"`
	CategoryID  int64      `json:"categoryId"`
	Category    string     `json:"category"`
	Status      string     `json:"status"`
	PublishAt   *time.Time `json:"publishAt"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	CreatedBy   int64      `json:"createdBy"`
	AuthorName  string     `json:"authorName"`
	IsRead      bool       `json:"isRead"`
	ReadCount   int64      `json:"readCount"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

