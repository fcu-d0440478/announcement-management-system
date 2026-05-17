package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"announcement-management-system/backend/internal/auth"
	"announcement-management-system/backend/internal/db"
	"announcement-management-system/backend/internal/models"
)

type Server struct {
	Store      *db.Store
	JWTSecret  string
	CORSOrigin string
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("POST /api/login", s.login)

	protected := http.NewServeMux()
	protected.HandleFunc("GET /me", s.me)
	protected.HandleFunc("GET /users", s.users)
	protected.HandleFunc("GET /categories", s.categories)
	protected.HandleFunc("POST /categories", s.createCategory)
	protected.HandleFunc("GET /announcements", s.announcements)
	protected.HandleFunc("POST /announcements", s.createAnnouncement)
	protected.HandleFunc("GET /announcements/{id}", s.announcement)
	protected.HandleFunc("PUT /announcements/{id}", s.updateAnnouncement)
	protected.HandleFunc("DELETE /announcements/{id}", s.deleteAnnouncement)
	protected.HandleFunc("POST /announcements/{id}/read", s.markRead)
	mux.Handle("/api/", http.StripPrefix("/api", auth.Middleware(s.JWTSecret, protected)))

	return s.cors(s.logging(mux))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	user, err := s.Store.FindUserByUsername(r.Context(), req.Username)
	if err != nil || !auth.CheckPassword(user.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	token, err := auth.IssueToken(s.JWTSecret, user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"token": token, "user": user})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r)
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r)
	if !auth.IsAdmin(user.Role) {
		writeError(w, http.StatusForbidden, "admin only")
		return
	}
	users, err := s.Store.Users(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load users")
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) categories(w http.ResponseWriter, r *http.Request) {
	categories, err := s.Store.Categories(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load categories")
		return
	}
	writeJSON(w, http.StatusOK, categories)
}

func (s *Server) createCategory(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r)
	if !auth.IsAdmin(user.Role) {
		writeError(w, http.StatusForbidden, "admin only")
		return
	}
	var category models.Category
	if err := json.NewDecoder(r.Body).Decode(&category); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	category.Name = strings.TrimSpace(category.Name)
	category.Description = strings.TrimSpace(category.Description)
	if category.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	created, err := s.Store.CreateCategory(r.Context(), category)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to create category")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) announcements(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r)
	categoryID, _ := strconv.ParseInt(r.URL.Query().Get("categoryId"), 10, 64)
	readState := strings.TrimSpace(r.URL.Query().Get("read"))
	if readState == "" && r.URL.Query().Get("unread") == "true" {
		readState = "unread"
	}
	filter := db.AnnouncementFilter{
		UserID:     user.UserID,
		Role:       user.Role,
		Query:      strings.TrimSpace(r.URL.Query().Get("q")),
		CategoryID: categoryID,
		Status:     strings.TrimSpace(r.URL.Query().Get("status")),
		ReadState:  readState,
	}
	announcements, err := s.Store.Announcements(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load announcements")
		return
	}
	writeJSON(w, http.StatusOK, announcements)
}

func (s *Server) announcement(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r)
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	announcement, err := s.Store.Announcement(r.Context(), id, user.UserID, user.Role)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeError(w, status, "announcement not found")
		return
	}
	writeJSON(w, http.StatusOK, announcement)
}

func (s *Server) createAnnouncement(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r)
	if !auth.CanManage(user.Role) {
		writeError(w, http.StatusForbidden, "admin or editor only")
		return
	}
	var announcement models.Announcement
	if err := json.NewDecoder(r.Body).Decode(&announcement); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	normalizeAnnouncement(&announcement)
	if err := db.ValidateAnnouncement(announcement); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	announcement.CreatedBy = user.UserID
	created, err := s.Store.CreateAnnouncement(r.Context(), announcement)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to create announcement")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) updateAnnouncement(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r)
	if !auth.CanManage(user.Role) {
		writeError(w, http.StatusForbidden, "admin or editor only")
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var announcement models.Announcement
	if err := json.NewDecoder(r.Body).Decode(&announcement); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	normalizeAnnouncement(&announcement)
	if err := db.ValidateAnnouncement(announcement); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Store.UpdateAnnouncement(r.Context(), id, announcement); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeError(w, status, "failed to update announcement")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) deleteAnnouncement(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r)
	if !auth.CanManage(user.Role) {
		writeError(w, http.StatusForbidden, "admin or editor only")
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.Store.DeleteAnnouncement(r.Context(), id); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeError(w, status, "failed to delete announcement")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) markRead(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r)
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := s.Store.Announcement(r.Context(), id, user.UserID, user.Role); err != nil {
		writeError(w, http.StatusNotFound, "announcement not found")
		return
	}
	if err := s.Store.MarkRead(r.Context(), id, user.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mark read")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "read"})
}

func normalizeAnnouncement(announcement *models.Announcement) {
	announcement.Title = strings.TrimSpace(announcement.Title)
	announcement.Content = strings.TrimSpace(announcement.Content)
	announcement.Status = strings.TrimSpace(announcement.Status)
	if announcement.Status == "" {
		announcement.Status = "draft"
	}
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (s.CORSOrigin == "*" || origin == s.CORSOrigin || strings.Contains(s.CORSOrigin, origin)) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
