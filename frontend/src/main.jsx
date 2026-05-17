import React, { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  Bell,
  CalendarClock,
  Check,
  Edit3,
  Eye,
  Filter,
  LogOut,
  Megaphone,
  Plus,
  Search,
  Shield,
  Trash2,
  UserRound,
  X,
} from "lucide-react";
import { createApi } from "./api/client";
import "./styles/app.css";

const emptyForm = {
  title: "",
  content: "",
  categoryId: "",
  status: "draft",
  publishAt: "",
  expiresAt: "",
};

function App() {
  const [token, setToken] = useState(localStorage.getItem("token") || "");
  const [user, setUser] = useState(() => JSON.parse(localStorage.getItem("user") || "null"));
  const api = useMemo(() => createApi(() => token), [token]);

  if (!token || !user) {
    return <Login onLogin={(session) => {
      localStorage.setItem("token", session.token);
      localStorage.setItem("user", JSON.stringify(session.user));
      setToken(session.token);
      setUser(session.user);
    }} api={api} />;
  }

  return <Dashboard api={api} user={user} onLogout={() => {
    localStorage.clear();
    setToken("");
    setUser(null);
  }} />;
}

function Login({ api, onLogin }) {
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("admin123");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function submit(e) {
    e.preventDefault();
    setLoading(true);
    setError("");
    try {
      onLogin(await api.login(username, password));
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="login-page">
      <section className="login-panel">
        <div className="brand-mark"><Megaphone size={34} /></div>
        <h1>公告管理系統</h1>
        <p>企業內部公告、排程發布與已讀追蹤。</p>
        <form onSubmit={submit} className="login-form">
          <label>帳號<input value={username} onChange={(e) => setUsername(e.target.value)} /></label>
          <label>密碼<input type="password" value={password} onChange={(e) => setPassword(e.target.value)} /></label>
          {error && <div className="error">{error}</div>}
          <button disabled={loading}>{loading ? "登入中..." : "登入"}</button>
        </form>
        <div className="demo-users">
          <span>admin/admin123</span>
          <span>editor/editor123</span>
          <span>user/user123</span>
        </div>
      </section>
    </main>
  );
}

function Dashboard({ api, user, onLogout }) {
  const canManage = user.role === "admin" || user.role === "editor";
  const [announcements, setAnnouncements] = useState([]);
  const [categories, setCategories] = useState([]);
  const [users, setUsers] = useState([]);
  const [filters, setFilters] = useState({ q: "", categoryId: "", status: "", unread: "false" });
  const [searchText, setSearchText] = useState("");
  const [form, setForm] = useState(emptyForm);
  const [editing, setEditing] = useState(null);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const displayedAnnouncements = announcements.filter((item) => {
    const terms = searchText.trim().toLowerCase().split(/\s+/).filter(Boolean);
    if (terms.length === 0) return true;
    const haystack = `${item.title} ${item.content}`.toLowerCase();
    return terms.every((term) => haystack.includes(term));
  });

  async function load() {
    setError("");
    try {
      const params = Object.fromEntries(Object.entries(filters).filter(([, value]) => value !== ""));
      const [announcementData, categoryData] = await Promise.all([
        api.announcements(params),
        api.categories(),
      ]);
      setAnnouncements(announcementData);
      setCategories(categoryData);
      if (user.role === "admin") setUsers(await api.users());
    } catch (err) {
      setError(err.message);
    }
  }

  useEffect(() => { load(); }, [filters.q, filters.categoryId, filters.status, filters.unread]);

  function applySearch(e) {
    e?.preventDefault();
    setFilters({ ...filters, q: searchText.trim() });
  }

  function clearSearch() {
    setSearchText("");
    setFilters({ ...filters, q: "" });
  }

  function edit(item) {
    setEditing(item.id);
    setForm({
      title: item.title,
      content: item.content,
      categoryId: String(item.categoryId),
      status: item.status,
      publishAt: toInputDate(item.publishAt),
      expiresAt: toInputDate(item.expiresAt),
    });
  }

  async function save(e) {
    e.preventDefault();
    setError("");
    const payload = {
      ...form,
      categoryId: Number(form.categoryId),
      publishAt: form.publishAt ? new Date(form.publishAt).toISOString() : null,
      expiresAt: form.expiresAt ? new Date(form.expiresAt).toISOString() : null,
    };
    try {
      if (editing) {
        await api.updateAnnouncement(editing, payload);
        setMessage("公告已更新");
      } else {
        await api.createAnnouncement(payload);
        setMessage("公告已建立");
      }
      setEditing(null);
      setForm(emptyForm);
      await load();
    } catch (err) {
      setError(err.message);
    }
  }

  async function remove(id) {
    if (!confirm("確定刪除此公告？")) return;
    await api.deleteAnnouncement(id);
    await load();
  }

  async function markRead(id) {
    await api.markRead(id);
    await load();
  }

  return (
    <main className="app-shell">
      <aside className="sidebar">
        <div className="brand"><Megaphone /><span>AMS</span></div>
        <div className="profile">
          <UserRound />
          <div><strong>{user.name}</strong><span>{roleLabel(user.role)}</span></div>
        </div>
        <button className="ghost" onClick={onLogout}><LogOut size={18} />登出</button>
      </aside>

      <section className="workspace">
        <header className="topbar">
          <div>
            <h1>公告中心</h1>
            <p>{canManage ? "管理發布、排程與已讀狀態" : "查看公司公告並回報已讀"}</p>
          </div>
          <div className="stat-grid">
            <Stat icon={<Bell />} label="總公告" value={displayedAnnouncements.length} />
            <Stat icon={<Eye />} label="未讀" value={displayedAnnouncements.filter((a) => !a.isRead).length} />
            {user.role === "admin" && <Stat icon={<Shield />} label="使用者" value={users.length} />}
          </div>
        </header>

        <section className="filters">
          <form className="search-form" onSubmit={applySearch}>
            <div className="searchbox">
              <Search size={18} />
              <input
                placeholder="搜尋標題或內容"
                value={searchText}
                onChange={(e) => setSearchText(e.target.value)}
                autoComplete="off"
              />
            </div>
            <button type="submit"><Search size={17} />搜尋</button>
            {filters.q && <button type="button" className="ghost icon-only" onClick={clearSearch} title="清除搜尋"><X size={18} /></button>}
          </form>
          <select value={filters.categoryId} onChange={(e) => setFilters({ ...filters, categoryId: e.target.value })}>
            <option value="">全部分類</option>
            {categories.map((category) => <option key={category.id} value={category.id}>{category.name}</option>)}
          </select>
          {canManage && (
            <select value={filters.status} onChange={(e) => setFilters({ ...filters, status: e.target.value })}>
              <option value="">全部狀態</option>
              <option value="draft">草稿</option>
              <option value="scheduled">排程</option>
              <option value="published">已發布</option>
              <option value="archived">封存</option>
            </select>
          )}
          <button className={filters.unread === "true" ? "active" : ""} onClick={() => setFilters({ ...filters, unread: filters.unread === "true" ? "false" : "true" })}>
            <Filter size={17} />未讀
          </button>
        </section>
        {(filters.q || searchText.trim()) && (
          <div className="search-summary">
            搜尋「{searchText.trim() || filters.q}」：{displayedAnnouncements.length} 筆結果
          </div>
        )}

        {(error || message) && <div className={error ? "error banner" : "success banner"}>{error || message}</div>}

        <div className={canManage ? "content-grid" : "content-grid single"}>
          <section className="announcement-list">
            {displayedAnnouncements.map((item) => (
              <article className={`announcement ${item.isRead ? "" : "unread"}`} key={item.id}>
                <div className="announcement-head">
                  <div>
                    <span className={`status ${item.status}`}>{statusLabel(item.status)}</span>
                    <span className="category">{item.category}</span>
                  </div>
                  <time>{formatDate(item.publishAt || item.createdAt)}</time>
                </div>
                <h2>{item.title}</h2>
                <p>{item.content}</p>
                <footer>
                  <span>{item.authorName} · 已讀 {item.readCount}</span>
                  <div className="actions">
                    {!item.isRead && <button onClick={() => markRead(item.id)}><Check size={16} />已讀</button>}
                    {canManage && <button onClick={() => edit(item)}><Edit3 size={16} />編輯</button>}
                    {canManage && <button className="danger" onClick={() => remove(item.id)}><Trash2 size={16} />刪除</button>}
                  </div>
                </footer>
              </article>
            ))}
            {displayedAnnouncements.length === 0 && <div className="empty">目前沒有符合條件的公告，請調整搜尋文字或篩選條件。</div>}
          </section>

          {canManage && (
            <section className="editor-panel">
              <h2><Plus size={20} />{editing ? "編輯公告" : "新增公告"}</h2>
              <form onSubmit={save} className="editor-form">
                <label>標題<input value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} required /></label>
                <label>分類<select value={form.categoryId} onChange={(e) => setForm({ ...form, categoryId: e.target.value })} required>
                  <option value="">選擇分類</option>
                  {categories.map((category) => <option key={category.id} value={category.id}>{category.name}</option>)}
                </select></label>
                <label>狀態<select value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value })}>
                  <option value="draft">草稿</option>
                  <option value="scheduled">排程</option>
                  <option value="published">已發布</option>
                  <option value="archived">封存</option>
                </select></label>
                <label><CalendarClock size={16} />發布時間<input type="datetime-local" value={form.publishAt} onChange={(e) => setForm({ ...form, publishAt: e.target.value })} /></label>
                <label>到期時間<input type="datetime-local" value={form.expiresAt} onChange={(e) => setForm({ ...form, expiresAt: e.target.value })} /></label>
                <label>內容<textarea rows="8" value={form.content} onChange={(e) => setForm({ ...form, content: e.target.value })} required /></label>
                <div className="form-actions">
                  <button>{editing ? "更新" : "建立"}</button>
                  {editing && <button type="button" className="ghost" onClick={() => { setEditing(null); setForm(emptyForm); }}><X size={16} />取消</button>}
                </div>
              </form>
              {user.role === "admin" && <CategoryCreator api={api} onCreated={load} />}
            </section>
          )}
        </div>
      </section>
    </main>
  );
}

function CategoryCreator({ api, onCreated }) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  async function submit(e) {
    e.preventDefault();
    await api.createCategory({ name, description });
    setName("");
    setDescription("");
    await onCreated();
  }
  return (
    <form className="category-form" onSubmit={submit}>
      <h3>新增分類</h3>
      <input placeholder="分類名稱" value={name} onChange={(e) => setName(e.target.value)} required />
      <input placeholder="描述" value={description} onChange={(e) => setDescription(e.target.value)} />
      <button>新增分類</button>
    </form>
  );
}

function Stat({ icon, label, value }) {
  return <div className="stat">{icon}<span>{label}</span><strong>{value}</strong></div>;
}

function roleLabel(role) {
  return { admin: "管理員", editor: "編輯", user: "一般使用者" }[role] || role;
}

function statusLabel(status) {
  return { draft: "草稿", scheduled: "排程", published: "發布", archived: "封存" }[status] || status;
}

function toInputDate(value) {
  if (!value) return "";
  const date = new Date(value);
  date.setMinutes(date.getMinutes() - date.getTimezoneOffset());
  return date.toISOString().slice(0, 16);
}

function formatDate(value) {
  if (!value) return "未設定";
  return new Intl.DateTimeFormat("zh-TW", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

createRoot(document.getElementById("root")).render(<App />);
