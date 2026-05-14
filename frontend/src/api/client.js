const API_URL = import.meta.env.VITE_API_URL || "http://localhost:8080/api";

export class ApiError extends Error {
  constructor(message, status) {
    super(message);
    this.status = status;
  }
}

export function createApi(getToken) {
  async function request(path, options = {}) {
    const headers = { "Content-Type": "application/json", ...(options.headers || {}) };
    const token = getToken();
    if (token) headers.Authorization = `Bearer ${token}`;
    const res = await fetch(`${API_URL}${path}`, { ...options, headers });
    if (res.status === 204) return null;
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new ApiError(data.error || "API request failed", res.status);
    return data;
  }

  return {
    login: (username, password) =>
      request("/login", { method: "POST", body: JSON.stringify({ username, password }) }),
    me: () => request("/me"),
    users: () => request("/users"),
    categories: () => request("/categories"),
    createCategory: (payload) => request("/categories", { method: "POST", body: JSON.stringify(payload) }),
    announcements: (params) => request(`/announcements?${new URLSearchParams(params)}`),
    createAnnouncement: (payload) => request("/announcements", { method: "POST", body: JSON.stringify(payload) }),
    updateAnnouncement: (id, payload) => request(`/announcements/${id}`, { method: "PUT", body: JSON.stringify(payload) }),
    deleteAnnouncement: (id) => request(`/announcements/${id}`, { method: "DELETE" }),
    markRead: (id) => request(`/announcements/${id}/read`, { method: "POST" }),
  };
}

