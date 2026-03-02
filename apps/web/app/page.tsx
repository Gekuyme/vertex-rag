"use client";

import { FormEvent, useEffect, useMemo, useState } from "react";

const APIBaseURL = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080";
const accessTokenStorageKey = "vertex_access_token";

type User = {
  id: string;
  org_id: string;
  email: string;
  role_id: number;
  role_name: string;
  permissions: string[];
  status: string;
  created_at: string;
};

type Role = {
  id: number;
  name: string;
  permissions: string[];
};

type DocumentEntry = {
  id: string;
  title: string;
  filename: string;
  mime: string;
  status: string;
  allowed_role_ids: number[];
  created_at: string;
};

type AuthResponse = {
  access_token: string;
  expires_in: number;
  user: User;
};

type RequestOptions = {
  method?: "GET" | "POST" | "PATCH";
  body?: unknown;
  token?: string;
};

export default function HomePage() {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [token, setToken] = useState<string>("");
  const [user, setUser] = useState<User | null>(null);
  const [roles, setRoles] = useState<Role[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [documents, setDocuments] = useState<DocumentEntry[]>([]);
  const [draftRoleByUser, setDraftRoleByUser] = useState<Record<string, number>>({});
  const [selectedRoleIDs, setSelectedRoleIDs] = useState<number[]>([]);
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [documentTitle, setDocumentTitle] = useState("");
  const [isBusy, setIsBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  const [organizationName, setOrganizationName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");

  const canManageUsers = useMemo(
    () => Boolean(user?.permissions?.includes("can_manage_users")),
    [user]
  );
  const canUploadDocs = useMemo(
    () => Boolean(user?.permissions?.includes("can_upload_docs")),
    [user]
  );

  useEffect(() => {
    const storedToken = window.localStorage.getItem(accessTokenStorageKey);
    if (!storedToken) {
      return;
    }

    setToken(storedToken);
    void hydrateSession(storedToken);
  }, []);

  async function hydrateSession(accessToken: string) {
    try {
      const profile = await apiRequest<User>("/me", { token: accessToken });
      setUser(profile);
      await loadWorkspace(accessToken, profile);
      setMessage(`Signed in as ${profile.email}`);
      setError("");
      return;
    } catch {
      try {
        const refreshed = await apiRequest<AuthResponse>("/auth/refresh", { method: "POST" });
        persistAccessToken(refreshed.access_token);
        setUser(refreshed.user);
        await loadWorkspace(refreshed.access_token, refreshed.user);
        setMessage(`Session restored for ${refreshed.user.email}`);
        setError("");
      } catch {
        clearSession();
      }
    }
  }

  async function loadWorkspace(accessToken: string, profile: User) {
    const roleResponse = await apiRequest<{ roles: Role[] }>("/roles", { token: accessToken });
    setRoles(roleResponse.roles);
    if (roleResponse.roles.length > 0) {
      setSelectedRoleIDs([profile.role_id]);
    }

    if (profile.permissions.includes("can_upload_docs")) {
      const documentResponse = await apiRequest<{ documents: DocumentEntry[] }>("/documents", {
        token: accessToken
      });
      setDocuments(documentResponse.documents);
    } else {
      setDocuments([]);
    }

    if (profile.permissions.includes("can_manage_users")) {
      const userResponse = await apiRequest<{ users: User[] }>("/admin/users", { token: accessToken });
      setUsers(userResponse.users);
      const nextDraftMap: Record<string, number> = {};
      userResponse.users.forEach((entry) => {
        nextDraftMap[entry.id] = entry.role_id;
      });
      setDraftRoleByUser(nextDraftMap);
    } else {
      setUsers([]);
      setDraftRoleByUser({});
    }
  }

  async function onSubmitAuth(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setIsBusy(true);
    setMessage("");
    setError("");
    try {
      let authResponse: AuthResponse;
      if (mode === "register") {
        authResponse = await apiRequest<AuthResponse>("/auth/register_owner", {
          method: "POST",
          body: {
            organization_name: organizationName,
            email,
            password
          }
        });
      } else {
        authResponse = await apiRequest<AuthResponse>("/auth/login", {
          method: "POST",
          body: { email, password }
        });
      }

      persistAccessToken(authResponse.access_token);
      setUser(authResponse.user);
      await loadWorkspace(authResponse.access_token, authResponse.user);
      setMessage(
        mode === "register"
          ? `Organization created. Signed in as ${authResponse.user.email}.`
          : `Signed in as ${authResponse.user.email}.`
      );
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setIsBusy(false);
    }
  }

  async function onChangeRole(userID: string) {
    const nextRoleID = draftRoleByUser[userID];
    if (!nextRoleID) {
      return;
    }
    setIsBusy(true);
    setMessage("");
    setError("");
    try {
      await apiRequest(`/admin/users/${userID}/role`, {
        method: "PATCH",
        token,
        body: { role_id: nextRoleID }
      });

      setUsers((currentUsers) =>
        currentUsers.map((entry) =>
          entry.id === userID
            ? {
                ...entry,
                role_id: nextRoleID,
                role_name: roles.find((role) => role.id === nextRoleID)?.name || entry.role_name
              }
            : entry
        )
      );
      setMessage("User role updated.");
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setIsBusy(false);
    }
  }

  async function onUploadDocument(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedFile) {
      setError("Select a file before upload.");
      return;
    }
    if (selectedRoleIDs.length === 0) {
      setError("Select at least one role.");
      return;
    }

    setIsBusy(true);
    setMessage("");
    setError("");
    try {
      const formData = new FormData();
      formData.append("file", selectedFile);
      if (documentTitle.trim() !== "") {
        formData.append("title", documentTitle.trim());
      }
      selectedRoleIDs.forEach((roleID) => {
        formData.append("allowed_role_ids", String(roleID));
      });

      await apiRequestMultipart<DocumentEntry>("/documents/upload", formData, token);
      if (user) {
        await loadWorkspace(token, user);
      }

      setSelectedFile(null);
      setDocumentTitle("");
      setMessage("Document uploaded and indexed as uploaded.");
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setIsBusy(false);
    }
  }

  function toggleUploadRole(roleID: number) {
    setSelectedRoleIDs((current) =>
      current.includes(roleID) ? current.filter((id) => id !== roleID) : [...current, roleID]
    );
  }

  async function onLogout() {
    setIsBusy(true);
    setMessage("");
    setError("");
    try {
      await apiRequest("/auth/logout", { method: "POST" });
      clearSession();
      setMessage("Signed out.");
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setIsBusy(false);
    }
  }

  function persistAccessToken(nextToken: string) {
    window.localStorage.setItem(accessTokenStorageKey, nextToken);
    setToken(nextToken);
  }

  function clearSession() {
    window.localStorage.removeItem(accessTokenStorageKey);
    setToken("");
    setUser(null);
    setRoles([]);
    setUsers([]);
    setDocuments([]);
    setDraftRoleByUser({});
    setSelectedRoleIDs([]);
    setSelectedFile(null);
    setDocumentTitle("");
  }

  return (
    <main className="shell">
      <section className="card">
        <div className="headerRow">
          <div>
            <p className="eyebrow">Vertex RAG</p>
            <h1>Owner and Admin Console</h1>
          </div>
          <p className="hint">
            API: <code>{APIBaseURL}</code>
          </p>
        </div>

        {!user && (
          <>
            <div className="toggleRow">
              <button
                type="button"
                className={`tabButton ${mode === "login" ? "active" : ""}`}
                onClick={() => setMode("login")}
              >
                Login
              </button>
              <button
                type="button"
                className={`tabButton ${mode === "register" ? "active" : ""}`}
                onClick={() => setMode("register")}
              >
                Register Owner
              </button>
            </div>

            <form className="formGrid" onSubmit={onSubmitAuth}>
              {mode === "register" && (
                <label>
                  Organization
                  <input
                    value={organizationName}
                    onChange={(event) => setOrganizationName(event.target.value)}
                    placeholder="Acme Inc."
                    required
                  />
                </label>
              )}

              <label>
                Email
                <input
                  type="email"
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  placeholder="owner@company.com"
                  required
                />
              </label>

              <label>
                Password
                <input
                  type="password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  minLength={8}
                  required
                />
              </label>

              <button type="submit" disabled={isBusy}>
                {isBusy ? "Processing..." : mode === "register" ? "Create organization" : "Login"}
              </button>
            </form>
          </>
        )}

        {user && (
          <>
            <div className="profileRow">
              <div>
                <p>
                  <strong>User:</strong> {user.email}
                </p>
                <p>
                  <strong>Role:</strong> {user.role_name}
                </p>
                <p>
                  <strong>Organization:</strong> {user.org_id}
                </p>
              </div>
              <button type="button" onClick={onLogout} disabled={isBusy}>
                Logout
              </button>
            </div>

            {canUploadDocs && (
              <div className="adminPanel">
                <h2>Knowledge Upload</h2>
                <form className="formGrid" onSubmit={onUploadDocument}>
                  <label>
                    Title (optional)
                    <input
                      value={documentTitle}
                      onChange={(event) => setDocumentTitle(event.target.value)}
                      placeholder="Policy handbook"
                    />
                  </label>

                  <label>
                    File
                    <input
                      type="file"
                      onChange={(event) => setSelectedFile(event.target.files?.[0] || null)}
                      required
                    />
                  </label>

                  <fieldset>
                    <legend>Allowed roles</legend>
                    <div className="rolesGrid">
                      {roles.map((role) => (
                        <label key={role.id} className="roleCheck">
                          <input
                            type="checkbox"
                            checked={selectedRoleIDs.includes(role.id)}
                            onChange={() => toggleUploadRole(role.id)}
                          />
                          <span>{role.name}</span>
                        </label>
                      ))}
                    </div>
                  </fieldset>

                  <button type="submit" disabled={isBusy}>
                    {isBusy ? "Uploading..." : "Upload document"}
                  </button>
                </form>

                <h3>Uploaded documents</h3>
                <table>
                  <thead>
                    <tr>
                      <th>Title</th>
                      <th>File</th>
                      <th>Status</th>
                      <th>Access roles</th>
                    </tr>
                  </thead>
                  <tbody>
                    {documents.map((entry) => (
                      <tr key={entry.id}>
                        <td>{entry.title}</td>
                        <td>{entry.filename}</td>
                        <td>{entry.status}</td>
                        <td>{entry.allowed_role_ids.join(", ")}</td>
                      </tr>
                    ))}
                    {documents.length === 0 && (
                      <tr>
                        <td colSpan={4}>No uploaded documents yet.</td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            )}

            {canManageUsers && (
              <div className="adminPanel">
                <h2>Users and Roles</h2>
                <table>
                  <thead>
                    <tr>
                      <th>Email</th>
                      <th>Role</th>
                      <th>Status</th>
                      <th>Assign</th>
                    </tr>
                  </thead>
                  <tbody>
                    {users.map((entry) => (
                      <tr key={entry.id}>
                        <td>{entry.email}</td>
                        <td>{entry.role_name}</td>
                        <td>{entry.status}</td>
                        <td>
                          <div className="assignRow">
                            <select
                              value={draftRoleByUser[entry.id] || entry.role_id}
                              onChange={(event) =>
                                setDraftRoleByUser((current) => ({
                                  ...current,
                                  [entry.id]: Number(event.target.value)
                                }))
                              }
                            >
                              {roles.map((role) => (
                                <option value={role.id} key={role.id}>
                                  {role.name}
                                </option>
                              ))}
                            </select>
                            <button
                              type="button"
                              onClick={() => void onChangeRole(entry.id)}
                              disabled={isBusy}
                            >
                              Save
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                    {users.length === 0 && (
                      <tr>
                        <td colSpan={4}>No users found.</td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            )}

            {!canUploadDocs && !canManageUsers && (
              <p className="notice">Current user has no management permissions.</p>
            )}
          </>
        )}

        {message && <p className="message success">{message}</p>}
        {error && <p className="message error">{error}</p>}
      </section>
    </main>
  );
}

async function apiRequest<T = unknown>(path: string, options: RequestOptions = {}): Promise<T> {
  const response = await fetch(`${APIBaseURL}${path}`, {
    method: options.method || "GET",
    headers: {
      "Content-Type": "application/json",
      ...(options.token ? { Authorization: `Bearer ${options.token}` } : {})
    },
    credentials: "include",
    body: options.body ? JSON.stringify(options.body) : undefined
  });

  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    const reason = typeof payload?.error === "string" ? payload.error : `Request failed: ${response.status}`;
    throw new Error(reason);
  }

  return payload as T;
}

async function apiRequestMultipart<T = unknown>(path: string, formData: FormData, token: string): Promise<T> {
  const response = await fetch(`${APIBaseURL}${path}`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`
    },
    credentials: "include",
    body: formData
  });

  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    const reason = typeof payload?.error === "string" ? payload.error : `Request failed: ${response.status}`;
    throw new Error(reason);
  }

  return payload as T;
}

function errorMessage(value: unknown): string {
  if (value instanceof Error) {
    return value.message;
  }

  return "Unexpected request error";
}

