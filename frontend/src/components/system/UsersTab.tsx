import { useCallback, useEffect, useState } from "react";
import { API_BASE_URL, authFetch } from "../../api/client";
import { useAuth } from "../../app/auth";
import { useSettings } from "../../app/settings";

type UserRecord = {
  id: string;
  email: string;
  name: string;
  avatar_url: string;
  role: "user" | "premium" | "admin";
  has_google: boolean;
  has_password: boolean;
  disabled: boolean;
  must_change_password: boolean;
  created_at: string;
  updated_at: string;
};

type UsersResponse = {
  data: UserRecord[] | null;
  total: number;
  page: number;
  limit: number;
};

const PAGE_SIZE = 50;
const SEARCH_DEBOUNCE_MS = 300;

export function UsersTab() {
  const { user: currentUser } = useAuth();
  const { theme } = useSettings();
  const light = theme === "light";

  // Theme-aware colors for badges and action buttons
  const colors = {
    adminBg: "rgba(239, 68, 68, 0.2)",
    adminText: light ? "#b91c1c" : "#fca5a5",
    premiumBg: "rgba(59, 130, 246, 0.2)",
    premiumText: light ? "#1d4ed8" : "#93c5fd",
    userBg: "rgba(107, 114, 128, 0.2)",
    userText: light ? "#374151" : "#d1d5db",
    activeBg: "rgba(34, 197, 94, 0.2)",
    activeText: light ? "#047857" : "#86efac",
    disabledBg: "rgba(220, 38, 38, 0.2)",
    disabledText: light ? "#b91c1c" : "#fca5a5",
    enableBg: "rgba(34, 197, 94, 0.15)",
    enableText: light ? "#047857" : "#86efac",
    dangerBg: "rgba(220, 38, 38, 0.15)",
    dangerText: light ? "#b91c1c" : "#fca5a5",
    infoBg: "rgba(59, 130, 246, 0.15)",
    infoText: light ? "#1d4ed8" : "#93c5fd",
    successMsgBg: "rgba(34, 197, 94, 0.12)",
    successMsgBorder: "rgba(34, 197, 94, 0.3)",
    successMsgText: light ? "#047857" : "#86efac",
    errorMsgBg: "rgba(220, 38, 38, 0.12)",
    errorMsgBorder: "rgba(220, 38, 38, 0.3)",
    errorMsgText: light ? "#b91c1c" : "#fca5a5",
  };

  // List state
  const [users, setUsers] = useState<UserRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Filter state
  const [searchTerm, setSearchTerm] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [roleFilter, setRoleFilter] = useState<string>("all");
  const [statusFilter, setStatusFilter] = useState<string>("all");

  // Detail view state
  const [selectedUser, setSelectedUser] = useState<UserRecord | null>(null);
  const [actionLoading, setActionLoading] = useState(false);
  const [actionMessage, setActionMessage] = useState<{
    type: "success" | "error";
    text: string;
  } | null>(null);

  // Debounce search input
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(searchTerm);
      setPage(1);
    }, SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [searchTerm]);

  // Fetch users
  const fetchUsers = useCallback(async () => {
    setLoading(true);
    setError(null);

    const params = new URLSearchParams({
      page: page.toString(),
      limit: PAGE_SIZE.toString(),
    });

    if (debouncedSearch) params.set("search", debouncedSearch);
    if (roleFilter !== "all") params.set("role", roleFilter);
    if (statusFilter !== "all") params.set("status", statusFilter);

    try {
      const res = await authFetch(`${API_BASE_URL}/system/users?${params}`);
      if (!res.ok) {
        throw new Error(`Failed to fetch users: ${res.statusText}`);
      }
      const json: UsersResponse = await res.json();
      setUsers(json.data ?? []);
      setTotal(json.total ?? 0);
    } catch (e) {
      console.error("Failed to fetch users", e);
      setError(e instanceof Error ? e.message : "Failed to fetch users");
      setUsers([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [page, debouncedSearch, roleFilter, statusFilter]);

  useEffect(() => {
    fetchUsers();
  }, [fetchUsers]);

  // Clear action message after 5 seconds
  useEffect(() => {
    if (!actionMessage) return;
    const timer = setTimeout(() => setActionMessage(null), 5000);
    return () => clearTimeout(timer);
  }, [actionMessage]);

  const handleRoleChange = async (userId: string, newRole: string) => {
    setActionLoading(true);
    setActionMessage(null);
    try {
      const res = await authFetch(`${API_BASE_URL}/system/users/${userId}/role`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ role: newRole }),
      });
      if (!res.ok) {
        const errorData = await res.json().catch(() => ({ error: res.statusText }));
        throw new Error(errorData.error || "Failed to update role");
      }
      const updatedUser: UserRecord = await res.json();
      setActionMessage({ type: "success", text: `Role updated to ${newRole}` });
      setSelectedUser(updatedUser);
      fetchUsers();
    } catch (e) {
      setActionMessage({
        type: "error",
        text: e instanceof Error ? e.message : "Failed to update role",
      });
    } finally {
      setActionLoading(false);
    }
  };

  const handleToggleDisabled = async (userId: string, disabled: boolean) => {
    setActionLoading(true);
    setActionMessage(null);
    try {
      const res = await authFetch(`${API_BASE_URL}/system/users/${userId}/status`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ disabled }),
      });
      if (!res.ok) {
        const errorData = await res.json().catch(() => ({ error: res.statusText }));
        throw new Error(errorData.error || "Failed to update status");
      }
      const updatedUser: UserRecord = await res.json();
      setActionMessage({
        type: "success",
        text: disabled ? "User disabled" : "User enabled",
      });
      setSelectedUser(updatedUser);
      fetchUsers();
    } catch (e) {
      setActionMessage({
        type: "error",
        text: e instanceof Error ? e.message : "Failed to update status",
      });
    } finally {
      setActionLoading(false);
    }
  };

  const handleForcePasswordReset = async (userId: string) => {
    setActionLoading(true);
    setActionMessage(null);
    try {
      const res = await authFetch(
        `${API_BASE_URL}/system/users/${userId}/force-password-reset`,
        { method: "POST" },
      );
      if (!res.ok) {
        const errorData = await res.json().catch(() => ({ error: res.statusText }));
        throw new Error(errorData.error || "Failed to force password reset");
      }
      const updatedUser: UserRecord = await res.json();
      setActionMessage({ type: "success", text: "Password reset required on next login" });
      setSelectedUser(updatedUser);
      fetchUsers();
    } catch (e) {
      setActionMessage({
        type: "error",
        text: e instanceof Error ? e.message : "Failed to force password reset",
      });
    } finally {
      setActionLoading(false);
    }
  };

  const formatDate = (dateStr: string) => {
    const date = new Date(dateStr);
    return date.toLocaleDateString("en-US", {
      year: "numeric",
      month: "short",
      day: "numeric",
    });
  };

  const isCurrentUser = (userId: string) => currentUser?.id === userId;

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "1.5rem", flex: 1, minHeight: 0 }}>
      {/* Filters and Search */}
      <div
        style={{
          padding: "1rem",
          backgroundColor: "var(--surface-color)",
          borderRadius: "12px",
          display: "flex",
          flexWrap: "wrap",
          gap: "0.75rem",
          alignItems: "center",
        }}
      >
        <input
          type="text"
          value={searchTerm}
          onChange={(e) => setSearchTerm(e.target.value)}
          placeholder="Search by email or name…"
          style={{
            flex: "1 1 200px",
            background: "var(--color-interactive-bg)",
            border: "1px solid var(--color-interactive-border)",
            borderRadius: "6px",
            color: "inherit",
            fontSize: "0.9rem",
            padding: "0.5rem 0.75rem",
            outline: "none",
          }}
          aria-label="Search users"
        />
        <select
          value={roleFilter}
          onChange={(e) => { setRoleFilter(e.target.value); setPage(1); }}
          style={{
            background: "var(--color-interactive-bg)",
            border: "1px solid var(--color-interactive-border)",
            borderRadius: "6px",
            color: "inherit",
            fontSize: "0.9rem",
            padding: "0.5rem 0.75rem",
            cursor: "pointer",
          }}
          aria-label="Filter by role"
        >
          <option value="all">All Roles</option>
          <option value="user">User</option>
          <option value="premium">Premium</option>
          <option value="admin">Admin</option>
        </select>
        <select
          value={statusFilter}
          onChange={(e) => { setStatusFilter(e.target.value); setPage(1); }}
          style={{
            background: "var(--color-interactive-bg)",
            border: "1px solid var(--color-interactive-border)",
            borderRadius: "6px",
            color: "inherit",
            fontSize: "0.9rem",
            padding: "0.5rem 0.75rem",
            cursor: "pointer",
          }}
          aria-label="Filter by status"
        >
          <option value="all">All Status</option>
          <option value="active">Active</option>
          <option value="disabled">Disabled</option>
        </select>
      </div>

      {/* Users Table */}
      <div
        style={{
          padding: "1rem",
          backgroundColor: "var(--surface-color)",
          borderRadius: "12px",
          display: "flex",
          flexDirection: "column",
          flex: 1,
          minHeight: "300px",
        }}
      >
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "1rem" }}>
          <h3 style={{ fontSize: "1rem", margin: 0 }}>
            Users <span style={{ opacity: 0.6 }}>({total})</span>
          </h3>
          {/* Pagination info */}
          <div style={{ fontSize: "0.82rem", opacity: 0.7 }}>
            Page {page} of {totalPages}
          </div>
        </div>

        {error && (
          <div
            style={{
              padding: "0.75rem",
              backgroundColor: "rgba(220, 38, 38, 0.1)",
              border: "1px solid rgba(220, 38, 38, 0.3)",
              borderRadius: "6px",
              color: colors.errorMsgText,
              marginBottom: "1rem",
            }}
          >
            {error}
          </div>
        )}

        {loading && users.length === 0 ? (
          <div style={{ padding: "2rem", textAlign: "center", opacity: 0.6, flex: 1, display: "flex", alignItems: "center", justifyContent: "center" }}>
            Loading users…
          </div>
        ) : users.length === 0 ? (
          <div style={{ padding: "2rem", textAlign: "center", opacity: 0.6, flex: 1, display: "flex", alignItems: "center", justifyContent: "center" }}>
            No users found matching the current filters.
          </div>
        ) : (
          <div style={{ overflowX: "auto", flex: 1 }}>
            <table className="data-table" style={{ fontSize: "0.9rem" }}>
              <thead>
                <tr>
                  <th style={{ textAlign: "left" }}>Name</th>
                  <th style={{ textAlign: "left" }}>Email</th>
                  <th>Role</th>
                  <th>Status</th>
                  <th>Created</th>
                </tr>
              </thead>
              <tbody>
                {users.map((user) => (
                  <tr
                    key={user.id}
                    onClick={() => setSelectedUser(user)}
                    style={{ cursor: "pointer" }}
                    className="hoverable-row"
                  >
                    <td style={{ textAlign: "left" }}>
                      {user.name || <span style={{ opacity: 0.5 }}>—</span>}
                    </td>
                    <td style={{ textAlign: "left" }}>{user.email}</td>
                    <td>
                      <span
                        style={{
                          padding: "0.25rem 0.5rem",
                          borderRadius: "4px",
                          fontSize: "0.8rem",
                          fontWeight: 500,
                          backgroundColor:
                            user.role === "admin"
                              ? colors.adminBg
                              : user.role === "premium"
                                ? colors.premiumBg
                                : colors.userBg,
                          color:
                            user.role === "admin"
                              ? colors.adminText
                              : user.role === "premium"
                                ? colors.premiumText
                                : colors.userText,
                        }}
                      >
                        {user.role}
                      </span>
                    </td>
                    <td>
                      <span
                        style={{
                          padding: "0.25rem 0.5rem",
                          borderRadius: "4px",
                          fontSize: "0.8rem",
                          fontWeight: 500,
                          backgroundColor: user.disabled
                            ? colors.disabledBg
                            : colors.activeBg,
                          color: user.disabled ? colors.disabledText : colors.activeText,
                        }}
                      >
                        {user.disabled ? "Disabled" : "Active"}
                      </span>
                    </td>
                    <td>{formatDate(user.created_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {/* Pagination */}
        <div
          style={{
            display: "flex",
            justifyContent: "flex-end",
            alignItems: "center",
            marginTop: "1rem",
            paddingTop: "0.75rem",
            borderTop: "1px solid var(--color-subtle-border)",
            gap: "0.5rem",
          }}
        >
          <button
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            disabled={page === 1}
            style={{
              padding: "0.4rem 0.8rem",
              fontSize: "0.85rem",
              borderRadius: "6px",
              border: "1px solid var(--color-interactive-border)",
              background: "var(--color-interactive-bg)",
              color: "inherit",
              cursor: page === 1 ? "not-allowed" : "pointer",
              opacity: page === 1 ? 0.4 : 1,
            }}
          >
            Previous
          </button>
          <button
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
            disabled={page >= totalPages}
            style={{
              padding: "0.4rem 0.8rem",
              fontSize: "0.85rem",
              borderRadius: "6px",
              border: "1px solid var(--color-interactive-border)",
              background: "var(--color-interactive-bg)",
              color: "inherit",
              cursor: page >= totalPages ? "not-allowed" : "pointer",
              opacity: page >= totalPages ? 0.4 : 1,
            }}
          >
            Next
          </button>
        </div>
      </div>

      {/* User Detail Modal */}
      {selectedUser && (
        <div
          style={{
            position: "fixed",
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            backgroundColor: "rgba(0, 0, 0, 0.6)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            zIndex: 1000,
            padding: "1rem",
          }}
          onClick={() => setSelectedUser(null)}
        >
          <div
            style={{
              backgroundColor: "var(--color-card-detail-bg)",
              border: "1px solid var(--color-card-detail-border)",
              borderRadius: "12px",
              padding: "1.5rem",
              maxWidth: "560px",
              width: "100%",
              maxHeight: "90vh",
              overflowY: "auto",
              boxShadow: "0 20px 60px rgba(0,0,0,0.5)",
            }}
            onClick={(e) => e.stopPropagation()}
          >
            <div
              style={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                marginBottom: "1.25rem",
                paddingBottom: "0.75rem",
                borderBottom: "1px solid var(--color-subtle-border)",
              }}
            >
              <h3 style={{ fontSize: "1.1rem", margin: 0 }}>User Details</h3>
              <button
                onClick={() => setSelectedUser(null)}
                style={{
                  background: "var(--color-interactive-bg)",
                  border: "1px solid var(--color-card-detail-border)",
                  borderRadius: "6px",
                  color: "inherit",
                  cursor: "pointer",
                  fontSize: "1rem",
                  lineHeight: 1,
                  padding: "0.25rem 0.5rem",
                }}
                aria-label="Close"
              >
                ✕
              </button>
            </div>

            {/* Action Message */}
            {actionMessage && (
              <div
                style={{
                  padding: "0.6rem 0.75rem",
                  backgroundColor:
                    actionMessage.type === "success"
                      ? colors.successMsgBg
                      : colors.errorMsgBg,
                  border: `1px solid ${
                    actionMessage.type === "success"
                      ? colors.successMsgBorder
                      : colors.errorMsgBorder
                  }`,
                  borderRadius: "6px",
                  color: actionMessage.type === "success" ? colors.successMsgText : colors.errorMsgText,
                  marginBottom: "1rem",
                  fontSize: "0.85rem",
                }}
              >
                {actionMessage.text}
              </div>
            )}

            {/* User Info Grid */}
            <div
              style={{
                display: "grid",
                gridTemplateColumns: "1fr 1fr",
                gap: "0.75rem",
                marginBottom: "1.25rem",
                fontSize: "0.88rem",
              }}
            >
              <div style={{ gridColumn: "1 / -1" }}>
                <div style={{ fontSize: "0.75rem", opacity: 0.5, marginBottom: "0.2rem" }}>Email</div>
                <div>{selectedUser.email}</div>
              </div>
              <div>
                <div style={{ fontSize: "0.75rem", opacity: 0.5, marginBottom: "0.2rem" }}>Name</div>
                <div>{selectedUser.name || <span style={{ opacity: 0.4 }}>—</span>}</div>
              </div>
              <div>
                <div style={{ fontSize: "0.75rem", opacity: 0.5, marginBottom: "0.2rem" }}>ID</div>
                <div style={{ fontSize: "0.78rem", fontFamily: "monospace", opacity: 0.8 }}>
                  {selectedUser.id.slice(0, 8)}…
                </div>
              </div>
              <div>
                <div style={{ fontSize: "0.75rem", opacity: 0.5, marginBottom: "0.2rem" }}>Created</div>
                <div>{formatDate(selectedUser.created_at)}</div>
              </div>
              <div>
                <div style={{ fontSize: "0.75rem", opacity: 0.5, marginBottom: "0.2rem" }}>Updated</div>
                <div>{formatDate(selectedUser.updated_at)}</div>
              </div>
              <div>
                <div style={{ fontSize: "0.75rem", opacity: 0.5, marginBottom: "0.2rem" }}>Auth Methods</div>
                <div>
                  {[
                    selectedUser.has_google && "Google",
                    selectedUser.has_password && "Password",
                  ]
                    .filter(Boolean)
                    .join(" + ") || "None"}
                </div>
              </div>
              <div>
                <div style={{ fontSize: "0.75rem", opacity: 0.5, marginBottom: "0.2rem" }}>Must Change Password</div>
                <div>{selectedUser.must_change_password ? "Yes" : "No"}</div>
              </div>
            </div>

            {/* Management Actions */}
            <div
              style={{
                display: "flex",
                flexDirection: "column",
                gap: "0.75rem",
                paddingTop: "1rem",
                borderTop: "1px solid var(--color-subtle-border)",
              }}
            >
              {/* Role Selector */}
              <div>
                <label
                  style={{ fontSize: "0.82rem", fontWeight: 500, marginBottom: "0.35rem", display: "block" }}
                >
                  Role
                </label>
                <select
                  value={selectedUser.role}
                  onChange={(e) => handleRoleChange(selectedUser.id, e.target.value)}
                  disabled={isCurrentUser(selectedUser.id) || actionLoading}
                  style={{
                    width: "100%",
                    background: "var(--color-interactive-bg)",
                    border: "1px solid var(--color-interactive-border)",
                    borderRadius: "6px",
                    color: "inherit",
                    fontSize: "0.88rem",
                    padding: "0.5rem 0.75rem",
                    cursor: isCurrentUser(selectedUser.id) ? "not-allowed" : "pointer",
                    opacity: isCurrentUser(selectedUser.id) ? 0.5 : 1,
                  }}
                >
                  <option value="user">User</option>
                  <option value="premium">Premium</option>
                  <option value="admin">Admin</option>
                </select>
                {isCurrentUser(selectedUser.id) && (
                  <div style={{ fontSize: "0.72rem", opacity: 0.5, marginTop: "0.2rem" }}>
                    You cannot change your own role
                  </div>
                )}
              </div>

              {/* Disable/Enable Toggle */}
              <div>
                <label
                  style={{ fontSize: "0.82rem", fontWeight: 500, marginBottom: "0.35rem", display: "block" }}
                >
                  Account Status
                </label>
                <button
                  onClick={() => handleToggleDisabled(selectedUser.id, !selectedUser.disabled)}
                  disabled={isCurrentUser(selectedUser.id) || actionLoading}
                  style={{
                    width: "100%",
                    padding: "0.5rem 1rem",
                    fontSize: "0.88rem",
                    borderRadius: "6px",
                    border: "1px solid var(--color-interactive-border)",
                    background: selectedUser.disabled
                      ? colors.enableBg
                      : colors.dangerBg,
                    color: selectedUser.disabled ? colors.enableText : colors.dangerText,
                    cursor: isCurrentUser(selectedUser.id) ? "not-allowed" : "pointer",
                    opacity: isCurrentUser(selectedUser.id) || actionLoading ? 0.5 : 1,
                  }}
                >
                  {selectedUser.disabled ? "Enable Account" : "Disable Account"}
                </button>
                {isCurrentUser(selectedUser.id) && (
                  <div style={{ fontSize: "0.72rem", opacity: 0.5, marginTop: "0.2rem" }}>
                    You cannot disable your own account
                  </div>
                )}
              </div>

              {/* Force Password Reset */}
              {selectedUser.has_password && (
                <div>
                  <label
                    style={{ fontSize: "0.82rem", fontWeight: 500, marginBottom: "0.35rem", display: "block" }}
                  >
                    Password Reset
                  </label>
                  <button
                    onClick={() => handleForcePasswordReset(selectedUser.id)}
                    disabled={actionLoading}
                    style={{
                      width: "100%",
                      padding: "0.5rem 1rem",
                      fontSize: "0.88rem",
                      borderRadius: "6px",
                      border: "1px solid var(--color-interactive-border)",
                      background: colors.infoBg,
                      color: colors.infoText,
                      cursor: actionLoading ? "not-allowed" : "pointer",
                      opacity: actionLoading ? 0.5 : 1,
                    }}
                  >
                    Force Password Reset on Next Login
                  </button>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
