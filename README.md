# 公告管理系統

企業內部公告管理系統，包含使用者登入、公告 CRUD、分類、角色權限、已讀未讀、搜尋與排程發布。

## 技術與版本

| 區塊      | 技術                        |
| --------- | --------------------------- |
| Frontend  | React + Vite + lucide-react |
| Backend   | Go 1.22, net/http           |
| Database  | PostgreSQL 16               |
| Auth      | JWT + bcrypt                |
| Container | Docker Compose              |

## 專案架構

```text
.
├── backend
│   ├── cmd/api              # API 入口
│   └── internal
│       ├── auth             # JWT、密碼與角色判斷
│       ├── config           # 環境變數
│       ├── db               # PostgreSQL migration、seed、資料存取
│       ├── handlers         # HTTP API handlers
│       └── models           # 資料模型
├── frontend
│   └── src                  # React UI 與 API client
├── docker-compose.yml
└── README.md
```

## 資料庫設計

### users

| 欄位          | 說明                      |
| ------------- | ------------------------- |
| id            | 使用者 ID                 |
| username      | 登入帳號，唯一            |
| password_hash | bcrypt 密碼雜湊           |
| role          | `admin`、`editor`、`user` |
| name          | 顯示名稱                  |

### categories

| 欄位        | 說明           |
| ----------- | -------------- |
| id          | 分類 ID        |
| name        | 分類名稱，唯一 |
| description | 分類描述       |

### announcements

| 欄位                    | 說明                                          |
| ----------------------- | --------------------------------------------- |
| id                      | 公告 ID                                       |
| title                   | 標題                                          |
| content                 | 內容                                          |
| category_id             | 分類                                          |
| status                  | `draft`、`scheduled`、`published`、`archived` |
| publish_at              | 發布時間，可用於排程                          |
| expires_at              | 到期時間                                      |
| created_by              | 建立者                                        |
| updated_by              | 最後編輯者                                    |
| deleted_at              | 軟刪除時間，未刪除為 `NULL`                   |
| created_at / updated_at | 建立與更新時間                                |

### announcement_reads

| 欄位            | 說明      |
| --------------- | --------- |
| announcement_id | 公告 ID   |
| user_id         | 使用者 ID |
| read_at         | 已讀時間  |

### audit_logs

| 欄位        | 說明                            |
| ----------- | ------------------------------- |
| id          | 操作紀錄 ID                     |
| user_id     | 操作者                          |
| action      | `create`、`update`、`delete`    |
| target_type | 目標類型，目前為 `announcement` |
| target_id   | 目標 ID                         |
| created_at  | 操作時間                        |

## 如何啟動

需要 Docker 與 Docker Compose。

```bash
docker compose up --build
```

啟動後：

- Frontend: http://localhost:5173
- Backend health check: http://localhost:8080/api/health
- PostgreSQL: localhost:5432
- 首次啟動會自動建立資料表，並建立中文預設分類、測試帳號與 demo 公告。

預設測試帳號：

| 角色       | 帳號   | 密碼      |
| ---------- | ------ | --------- |
| 管理員     | admin  | admin123  |
| 編輯       | editor | editor123 |
| 一般使用者 | user   | user123   |

## API 摘要

| Method | Path                           | 說明                                               |
| ------ | ------------------------------ | -------------------------------------------------- |
| GET    | `/api/health`                  | 後端健康檢查                                       |
| POST   | `/api/login`                   | 登入並取得 JWT                                     |
| GET    | `/api/me`                      | 目前登入使用者                                     |
| GET    | `/api/users`                   | 使用者列表，限 admin                               |
| GET    | `/api/categories`              | 分類列表                                           |
| POST   | `/api/categories`              | 新增分類，限 admin                                 |
| GET    | `/api/announcements`           | 公告列表，支援 `q`、`categoryId`、`status`、`read` |
| POST   | `/api/announcements`           | 新增公告，限 admin/editor                          |
| GET    | `/api/announcements/{id}`      | 公告詳細                                           |
| PUT    | `/api/announcements/{id}`      | 更新公告，限 admin/editor                          |
| DELETE | `/api/announcements/{id}`      | 軟刪除公告，限 admin/editor                        |
| POST   | `/api/announcements/{id}/read` | 標記已讀                                           |

`GET /api/announcements` 常用查詢參數：

| 參數       | 說明                                                           |
| ---------- | -------------------------------------------------------------- |
| q          | 搜尋標題或內容，支援多關鍵字                                   |
| categoryId | 依分類篩選                                                     |
| status     | 管理者可依 `draft`、`scheduled`、`published`、`archived` 篩選  |
| read       | 依閱讀狀態篩選，空值為全部，`unread` 為未讀取，`read` 為已讀取 |

## 實作範圍與取捨

已完成基本要求：

- 使用者登入與 JWT 驗證
- 角色權限差異：`admin` 可新增分類與看使用者列表，`admin/editor` 可管理公告，`user` 只能看已發布公告與標記已讀
- 公告 CRUD
- 公告分類
- 已讀 / 未讀與已讀數統計
- 公告搜尋與分類、狀態、已讀狀態篩選
- 排程發布與公告到期時間
- Audit Log 操作紀錄
- Soft Delete 軟刪除
- PostgreSQL 實際資料庫儲存
- Docker Compose 一鍵啟動前端、後端、資料庫

取捨：

- 使用 seed 建立測試帳號、中文分類與 demo 公告，尚未實作使用者管理 CRUD。
- 權限模型採簡單三角色，未做細緻 RBAC policy table。
- 前端以單頁 dashboard 為主，UI 以可操作與清楚驗證功能為優先。

## 加分項

我實作了二個真實產品中常見且重要的點：

1. Audit Log：在真實企業系統中，公告內容可能涉及重要通知，因此需要保留操作紀錄，以便追蹤異動來源與責任歸屬。本系統新增 `audit_logs` 機制，在公告新增、修改、刪除時記錄操作者、操作類型、目標公告與操作時間。
2. Soft Delete：真實產品通常不會直接永久刪除公告，避免法遵、稽核或歷史追蹤需要時資料已遺失。本系統刪除公告時只標記 `deleted_at`，一般查詢會排除已刪除公告，資料仍保留在資料庫中。

## AI 輔助使用情況

本專案使用 Open AI Codex 輔助規劃與實作初版架構、Go API、React UI、Docker Compose、README，以及後續功能調整。每次修改後皆透過建置或 API 測試檢查主要功能，並保留 Git commit 歷史供檢視。
