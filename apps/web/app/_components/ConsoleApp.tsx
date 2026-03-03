"use client";

import type { FormEvent } from "react";
import { useEffect, useMemo, useRef, useState } from "react";

const APIBaseURL = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080";
const accessTokenStorageKey = "vertex_access_token";

type ConsoleView = "chat" | "knowledge" | "users" | "account";
type SettingsTab = "knowledge" | "users" | "roles" | "account";

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
  is_default: boolean;
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

type UserSettings = {
  user_id: string;
  default_mode: "strict" | "unstrict";
  updated_at: string;
};

type Chat = {
  id: string;
  title: string;
  created_at: string;
  updated_at: string;
};

type Citation = {
  chunk_id: string;
  document_id: string;
  doc_title: string;
  doc_filename: string;
  snippet: string;
  page?: number;
  section?: string;
  vector_score?: number;
  text_score?: number;
  score?: number;
  metadata?: Record<string, unknown>;
};

type ChatMessage = {
  id: string;
  chat_id: string;
  user_id?: string | null;
  role: "user" | "assistant";
  mode: "strict" | "unstrict";
  content: string;
  citations: Citation[];
  created_at: string;
};

type RequestOptions = {
  method?: "GET" | "POST" | "PATCH" | "DELETE";
  body?: unknown;
  token?: string;
};

type StreamDonePayload = {
  mode: "strict" | "unstrict";
  user_message: ChatMessage;
  assistant_message: ChatMessage;
  citations: Citation[];
};

type ConsoleAppProps = {
  initialView?: ConsoleView;
};

const rolePermissionOptions = [
  { key: "can_upload_docs", label: "Загрузка документов" },
  { key: "can_manage_users", label: "Управление пользователями" },
  { key: "can_manage_roles", label: "Управление ролями" },
  { key: "can_manage_documents", label: "Управление документами" },
  { key: "can_toggle_web_search", label: "Web-search в unstrict" },
  { key: "can_use_unstrict", label: "Использование unstrict" }
] as const;

export default function ConsoleApp({ initialView = "chat" }: ConsoleAppProps) {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [view, setView] = useState<ConsoleView>(initialView);
  const [isNavOpen, setIsNavOpen] = useState(false);
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [isUploadModalOpen, setIsUploadModalOpen] = useState(false);
  const [settingsTab, setSettingsTab] = useState<SettingsTab>("account");

  const [token, setToken] = useState<string>("");
  const [user, setUser] = useState<User | null>(null);
  const [settings, setSettings] = useState<UserSettings | null>(null);

  const [roles, setRoles] = useState<Role[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [documents, setDocuments] = useState<DocumentEntry[]>([]);
  const [draftRoleByUser, setDraftRoleByUser] = useState<Record<string, number>>({});
  const [selectedRoleIDs, setSelectedRoleIDs] = useState<number[]>([]);
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [documentTitle, setDocumentTitle] = useState("");
  const [editingRoleID, setEditingRoleID] = useState<number | null>(null);
  const [roleDraftName, setRoleDraftName] = useState("");
  const [roleDraftPermissions, setRoleDraftPermissions] = useState<string[]>([]);

  const [chats, setChats] = useState<Chat[]>([]);
  const [activeChatID, setActiveChatID] = useState<string>("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [draftMessage, setDraftMessage] = useState("");
  const [messageMode, setMessageMode] = useState<"strict" | "unstrict">("strict");
  const [streamingAssistant, setStreamingAssistant] = useState("");
  const [isStreamingMessage, setIsStreamingMessage] = useState(false);
  const [isCitationPreviewOpen, setIsCitationPreviewOpen] = useState(false);
  const [activeCitation, setActiveCitation] = useState<Citation | null>(null);

  const composerRef = useRef<HTMLTextAreaElement | null>(null);
  const scrollRef = useRef<HTMLDivElement | null>(null);

  const [isBusy, setIsBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  const [organizationName, setOrganizationName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const modalAnimationMs = 200;

  const canManageUsers = useMemo(
    () => Boolean(user?.permissions?.includes("can_manage_users")),
    [user]
  );
  const canManageRoles = useMemo(
    () => Boolean(user?.permissions?.includes("can_manage_roles")),
    [user]
  );
  const canUploadDocs = useMemo(
    () => Boolean(user?.permissions?.includes("can_upload_docs")),
    [user]
  );
  const canToggleWebSearch = useMemo(
    () => Boolean(user?.permissions?.includes("can_toggle_web_search")),
    [user]
  );
  const isSettingsModalPresent = useModalPresence(isSettingsOpen, modalAnimationMs);
  const isUploadModalPresent = useModalPresence(isUploadModalOpen, modalAnimationMs);
  const isCitationPreviewPresent = useModalPresence(isCitationPreviewOpen, modalAnimationMs);

  useEffect(() => {
    const storedToken = window.localStorage.getItem(accessTokenStorageKey);
    if (!storedToken) {
      return;
    }

    setToken(storedToken);
    void hydrateSession(storedToken);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!user) {
      return;
    }

    // Sync chat mode selector with user default.
    if (settings?.default_mode) {
      setMessageMode(settings.default_mode);
    } else {
      setMessageMode("strict");
    }
  }, [user, settings?.default_mode]);

  useEffect(() => {
    // Keep the latest message visible when chatting.
    if (view !== "chat") {
      return;
    }

    const target = scrollRef.current;
    if (!target) {
      return;
    }
    target.scrollTop = target.scrollHeight;
  }, [messages, view]);

  useEffect(() => {
    if (!isSettingsOpen && !isUploadModalOpen && !isCitationPreviewOpen) {
      return;
    }

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setIsSettingsOpen(false);
        setIsUploadModalOpen(false);
        setIsCitationPreviewOpen(false);
      }
    }

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [isSettingsOpen, isUploadModalOpen, isCitationPreviewOpen]);

  async function hydrateSession(accessToken: string) {
    try {
      const profile = await apiRequest<User>("/me", { token: accessToken });
      setUser(profile);

      const nextSettings = await apiRequest<UserSettings>("/me/settings", { token: accessToken });
      setSettings(nextSettings);

      await loadWorkspace(accessToken, profile);
      await bootstrapChats(accessToken);

      setMessage(`Вход выполнен как ${profile.email}`);
      setError("");
      return;
    } catch {
      try {
        const refreshed = await apiRequest<AuthResponse>("/auth/refresh", { method: "POST" });
        persistAccessToken(refreshed.access_token);
        setUser(refreshed.user);

        const nextSettings = await apiRequest<UserSettings>("/me/settings", { token: refreshed.access_token });
        setSettings(nextSettings);

        await loadWorkspace(refreshed.access_token, refreshed.user);
        await bootstrapChats(refreshed.access_token);

        setMessage(`Сессия восстановлена для ${refreshed.user.email}`);
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

    // Docs can be visible to any signed-in user, but upload is permission gated.
    const documentResponse = await apiRequest<{ documents: DocumentEntry[] }>("/documents", {
      token: accessToken
    });
    setDocuments(documentResponse.documents);

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

  async function refreshChats(accessToken: string) {
    const chatResponse = await apiRequest<{ chats: Chat[] }>("/chats", { token: accessToken });
    setChats(chatResponse.chats);
    return chatResponse.chats;
  }

  async function bootstrapChats(accessToken: string) {
    const list = await refreshChats(accessToken);

    const preferredChatID = activeChatID || list[0]?.id || "";
    if (preferredChatID) {
      setActiveChatID(preferredChatID);
      await loadChatMessages(accessToken, preferredChatID);
      return;
    }

    const created = await apiRequest<Chat>("/chats", { method: "POST", token: accessToken, body: { title: "" } });
    setChats([created]);
    setActiveChatID(created.id);
    setMessages([]);
  }

  async function loadChatMessages(accessToken: string, chatID: string) {
    const response = await apiRequest<{ chat: Chat; messages: ChatMessage[] }>(`/chats/${chatID}/messages`, {
      token: accessToken
    });
    setMessages(response.messages);
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

      const nextSettings = await apiRequest<UserSettings>("/me/settings", { token: authResponse.access_token });
      setSettings(nextSettings);

      await loadWorkspace(authResponse.access_token, authResponse.user);
      await bootstrapChats(authResponse.access_token);

      setMessage(
        mode === "register"
          ? `Организация создана. Вход выполнен как ${authResponse.user.email}.`
          : `Вход выполнен как ${authResponse.user.email}.`
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
      setMessage("Роль пользователя обновлена.");
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setIsBusy(false);
    }
  }

  async function onUploadDocument(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedFile) {
      setError("Выберите файл перед загрузкой.");
      return;
    }
    if (selectedRoleIDs.length === 0) {
      setError("Выберите хотя бы одну роль.");
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
      setMessage("Документ загружен и отправлен в индексацию.");
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setIsBusy(false);
    }
  }

  async function onReingestDocument(documentID: string) {
    if (!token || !user) {
      return;
    }
    setIsBusy(true);
    setMessage("");
    setError("");
    try {
      await apiRequest(`/documents/${documentID}/reingest`, {
        method: "POST",
        token
      });
      await loadWorkspace(token, user);
      setMessage("Документ отправлен в переиндексацию.");
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setIsBusy(false);
    }
  }

  async function onReingestAllDocuments() {
    if (!token || !user) {
      return;
    }
    setIsBusy(true);
    setMessage("");
    setError("");
    try {
      const response = await apiRequest<{ scheduled_count: number; total_count: number }>("/documents/reingest_all", {
        method: "POST",
        token
      });
      await loadWorkspace(token, user);
      setMessage(`Переиндексация запланирована: ${response.scheduled_count}/${response.total_count}.`);
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setIsBusy(false);
    }
  }

  async function onCreateChat() {
    if (!token) {
      return;
    }
    setIsBusy(true);
    setMessage("");
    setError("");
    try {
      const created = await apiRequest<Chat>("/chats", { method: "POST", token, body: { title: "" } });
      setChats((current) => [created, ...current]);
      setActiveChatID(created.id);
      setMessages([]);
      setView("chat");
      setIsNavOpen(false);
      setMessage("Новый чат создан.");
      queueMicrotask(() => composerRef.current?.focus());
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setIsBusy(false);
    }
  }

  async function onSelectChat(chatID: string) {
    if (!token) {
      return;
    }
    if (chatID === activeChatID) {
      setView("chat");
      setIsNavOpen(false);
      return;
    }
    setIsBusy(true);
    setMessage("");
    setError("");
    try {
      setActiveChatID(chatID);
      await loadChatMessages(token, chatID);
      setView("chat");
      setIsNavOpen(false);
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setIsBusy(false);
    }
  }

  async function onSendMessage(event?: FormEvent<HTMLFormElement>) {
    event?.preventDefault();
    if (!token || !activeChatID) {
      return;
    }

    const content = draftMessage.trim();
    if (content === "") {
      return;
    }
    const optimisticMessageID = `local-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    const optimisticUserMessage: ChatMessage = {
      id: optimisticMessageID,
      chat_id: activeChatID,
      user_id: user?.id || null,
      role: "user",
      mode: messageMode,
      content,
      citations: [],
      created_at: new Date().toISOString()
    };

    setIsBusy(true);
    setIsStreamingMessage(true);
    setMessage("");
    setError("");
    setDraftMessage("");
    setStreamingAssistant("");
    setMessages((current) => [...current, optimisticUserMessage]);

    try {
      const payload: Record<string, unknown> = {
        content
      };

      // If mode is explicitly selected, send it.
      if (messageMode) {
        payload.mode = messageMode;
      }

      const response = await streamChatMessage(activeChatID, payload, token, {
        onUserMessage(nextMessage) {
          setMessages((current) => {
            let didReplaceOptimistic = false;
            const replaced = current.map((entry) => {
              if (entry.id === optimisticMessageID) {
                didReplaceOptimistic = true;
                return nextMessage;
              }
              return entry;
            });
            if (replaced.some((entry) => entry.id === nextMessage.id)) {
              return replaced;
            }
            return didReplaceOptimistic ? replaced : [...replaced, nextMessage];
          });
        },
        onAssistantDelta(delta) {
          setStreamingAssistant((current) => current + delta);
        }
      });

      setMessages((current) => {
        const replacedOptimistic = current.map((entry) =>
          entry.id === optimisticMessageID ? response.user_message : entry
        );
        const nextMessages = [...replacedOptimistic];
        if (!nextMessages.some((entry) => entry.id === response.user_message.id)) {
          nextMessages.push(response.user_message);
        }
        if (!nextMessages.some((entry) => entry.id === response.assistant_message.id)) {
          nextMessages.push(response.assistant_message);
        }
        return nextMessages;
      });
      setStreamingAssistant("");
      // Refresh list so sidebar ordering stays accurate, but keep active chat and local messages.
      await refreshChats(token);
    } catch (requestError) {
      setStreamingAssistant("");
      setMessages((current) => current.filter((entry) => entry.id !== optimisticMessageID));
      setError(errorMessage(requestError));
    } finally {
      setIsBusy(false);
      setIsStreamingMessage(false);
      queueMicrotask(() => composerRef.current?.focus());
    }
  }

  async function onDeleteChat() {
    if (!token || !activeChatID) {
      return;
    }

    if (!window.confirm("Удалить текущий чат? Это действие нельзя отменить.")) {
      return;
    }

    setIsBusy(true);
    setMessage("");
    setError("");
    try {
      await apiRequest(`/chats/${activeChatID}`, {
        method: "DELETE",
        token
      });

      const list = await refreshChats(token);
      if (list.length === 0) {
        const created = await apiRequest<Chat>("/chats", {
          method: "POST",
          token,
          body: { title: "" }
        });
        setChats([created]);
        setActiveChatID(created.id);
        setMessages([]);
      } else {
        const nextChatID = list[0].id;
        setActiveChatID(nextChatID);
        await loadChatMessages(token, nextChatID);
      }
      setMessage("Чат удалён.");
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setIsBusy(false);
    }
  }

  async function onUpdateDefaultMode(nextMode: "strict" | "unstrict") {
    if (!token) {
      return;
    }
    setIsBusy(true);
    setMessage("");
    setError("");
    try {
      const updated = await apiRequest<UserSettings>("/me/settings", {
        method: "PATCH",
        token,
        body: { default_mode: nextMode }
      });
      setSettings(updated);
      setMessage("Режим по умолчанию обновлён.");
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setIsBusy(false);
    }
  }

  function resetRoleDraft() {
    setEditingRoleID(null);
    setRoleDraftName("");
    setRoleDraftPermissions([]);
  }

  function onStartEditRole(role: Role) {
    setEditingRoleID(role.id);
    setRoleDraftName(role.name);
    setRoleDraftPermissions(role.permissions || []);
  }

  function toggleRoleDraftPermission(permission: string) {
    setRoleDraftPermissions((current) =>
      current.includes(permission)
        ? current.filter((entry) => entry !== permission)
        : [...current, permission]
    );
  }

  async function onSaveRole() {
    if (!token || !user || !canManageRoles) {
      return;
    }

    const cleanName = roleDraftName.trim();
    if (cleanName === "") {
      setError("Название роли обязательно.");
      return;
    }

    setIsBusy(true);
    setMessage("");
    setError("");
    try {
      if (editingRoleID) {
        await apiRequest(`/admin/roles/${editingRoleID}`, {
          method: "PATCH",
          token,
          body: {
            name: cleanName,
            permissions: roleDraftPermissions
          }
        });
        setMessage("Роль обновлена.");
      } else {
        await apiRequest("/admin/roles", {
          method: "POST",
          token,
          body: {
            name: cleanName,
            permissions: roleDraftPermissions
          }
        });
        setMessage("Роль создана.");
      }

      resetRoleDraft();
      await loadWorkspace(token, user);
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setIsBusy(false);
    }
  }

  async function onDeleteRole(role: Role) {
    if (!token || !user || !canManageRoles) {
      return;
    }
    if (role.is_default) {
      setError("Системные роли нельзя удалить.");
      return;
    }

    if (!window.confirm(`Удалить роль «${role.name}»?`)) {
      return;
    }

    setIsBusy(true);
    setMessage("");
    setError("");
    try {
      await apiRequest(`/admin/roles/${role.id}`, {
        method: "DELETE",
        token
      });
      if (editingRoleID === role.id) {
        resetRoleDraft();
      }
      await loadWorkspace(token, user);
      setMessage("Роль удалена.");
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setIsBusy(false);
    }
  }

  function onOpenCitationPreview(citation: Citation) {
    setActiveCitation(citation);
    setIsCitationPreviewOpen(true);
  }

  function closeCitationPreview() {
    setIsCitationPreviewOpen(false);
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
      setMessage("Вы вышли из аккаунта.");
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
    setSettings(null);
    setView(initialView);

    setRoles([]);
    setUsers([]);
    setDocuments([]);
    setDraftRoleByUser({});
    setSelectedRoleIDs([]);
    setSelectedFile(null);
    setDocumentTitle("");
    setEditingRoleID(null);
    setRoleDraftName("");
    setRoleDraftPermissions([]);

    setChats([]);
    setActiveChatID("");
    setMessages([]);
    setDraftMessage("");
    setStreamingAssistant("");
    setIsStreamingMessage(false);
  }

  const activeChat = useMemo(
    () => chats.find((chat) => chat.id === activeChatID) || null,
    [chats, activeChatID]
  );
  const activeCitationDocument = useMemo(() => {
    if (!activeCitation) {
      return null;
    }

    return documents.find((entry) => entry.id === activeCitation.document_id) || null;
  }, [activeCitation, documents]);

  const shellClassName = user ? "shell shellApp" : "shell";
  const cardClassName = user ? "card cardApp" : "card";

  return (
    <main className={shellClassName}>
      <section className={cardClassName}>
        {!user && (
          <div className="headerRow">
            <div>
              <h1>Вход</h1>
            </div>
            <p className="hint">Консоль владельца и администратора</p>
          </div>
        )}

        {!user && (
          <>
            <div className="toggleRow">
              <button
                type="button"
                className={`tabButton ${mode === "login" ? "active" : ""}`}
                onClick={() => setMode("login")}
              >
                Войти
              </button>
              <button
                type="button"
                className={`tabButton ${mode === "register" ? "active" : ""}`}
                onClick={() => setMode("register")}
              >
                Регистрация владельца
              </button>
            </div>

            <form className="formGrid" onSubmit={onSubmitAuth}>
              {mode === "register" && (
                <label>
                  Организация
                  <input
                    value={organizationName}
                    onChange={(event) => setOrganizationName(event.target.value)}
                    placeholder="ООО Пример"
                    required
                  />
                </label>
              )}

              <label>
                Почта
                <input
                  type="email"
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  placeholder="owner@company.com"
                  required
                />
              </label>

              <label>
                Пароль
                <input
                  type="password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  minLength={8}
                  required
                />
              </label>

              <button type="submit" className="btn btnPrimary" disabled={isBusy}>
                {isBusy ? "Обработка..." : mode === "register" ? "Создать организацию" : "Войти"}
              </button>
            </form>
          </>
        )}

        {user && (
          <div className="appShell">
            <aside className={`sidebar ${isNavOpen ? "open" : ""}`}>
              <div className="sidebarTop">
                <div className="sidebarBrand">
                  <div className="sidebarTitle">Vertex RAG</div>
                  <div className="sidebarSub">Защищённое рабочее пространство</div>
                </div>
              </div>

              <div className="sidebarSection">
                <div className="sidebarSectionHeader">
                  <div className="sidebarSectionTitle">Чаты</div>
                  <div className="iconBtnRow">
                    <button type="button" className="iconCreateBtn" onClick={() => void onCreateChat()} aria-label="Создать чат">
                      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="12" y1="5" x2="12" y2="19"></line><line x1="5" y1="12" x2="19" y2="12"></line></svg>
                    </button>
                  </div>
                </div>
                <div className="chatList" role="list">
                  {chats.map((chat) => (
                    <button
                      key={chat.id}
                      type="button"
                      className={`chatListItem ${chat.id === activeChatID ? "active" : ""}`}
                      onClick={() => void onSelectChat(chat.id)}
                      role="listitem"
                    >
                      <span className="chatListTitle">{chat.title}</span>
                    </button>
                  ))}
                  {chats.length === 0 && <div className="chatListEmpty">Пока нет чатов.</div>}
                </div>
              </div>

              <div className="sidebarFooter">
                <button
                  type="button"
                  className="btn btnSecondary btnSmall sidebarSettingsBtn"
                  aria-label="Открыть настройки"
                  onClick={() => {
                    setSettingsTab("account");
                    setIsSettingsOpen(true);
                  }}
                >
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="3"></circle><path d="M19.4 15A1.65 1.65 0 0 0 20 13.6a1.65 1.65 0 0 0-.6-1.2l-1.1-.9a1.65 1.65 0 0 1-.5-1.8l.5-1.3a1.65 1.65 0 0 0-.3-1.8 1.65 1.65 0 0 0-1.7-.5l-1.4.4a1.65 1.65 0 0 1-1.7-.5l-.9-1.1A1.65 1.65 0 0 0 12.4 4a1.65 1.65 0 0 0-1.2.6l-.9 1.1a1.65 1.65 0 0 1-1.8.5l-1.3-.5a1.65 1.65 0 0 0-1.8.3 1.65 1.65 0 0 0-.5 1.7l.4 1.4a1.65 1.65 0 0 1-.5 1.7L4 11.6a1.65 1.65 0 0 0-.6 1.2A1.65 1.65 0 0 0 4 14.2l1.1.9a1.65 1.65 0 0 1 .5 1.8l-.5 1.3a1.65 1.65 0 0 0 .3 1.8 1.65 1.65 0 0 0 1.7.5l1.4-.4a1.65 1.65 0 0 1 1.7.5l.9 1.1a1.65 1.65 0 0 0 1.2.6 1.65 1.65 0 0 0 1.2-.6l.9-1.1a1.65 1.65 0 0 1 1.8-.5l1.3.5a1.65 1.65 0 0 0 1.8-.3 1.65 1.65 0 0 0 .5-1.7l-.4-1.4a1.65 1.65 0 0 1 .5-1.7z"></path></svg>
                  <span>Настройки</span>
                </button>
              </div>
            </aside>

            <section className="mainPane">
              <header className="mainHeader">
                <button
                  type="button"
                  className="btn btnSecondary btnSmall navToggle"
                  onClick={() => setIsNavOpen((current) => !current)}
                >
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="3" y1="12" x2="21" y2="12"></line><line x1="3" y1="6" x2="21" y2="6"></line><line x1="3" y1="18" x2="21" y2="18"></line></svg>
                </button>

                <div className="mainHeaderTitle">
                  {view === "chat" && <span>{activeChat?.title || "Новый чат"}</span>}
                  {view === "knowledge" && <span>База знаний</span>}
                  {view === "users" && <span>Пользователи</span>}
                  {view === "account" && <span>Аккаунт</span>}
                </div>
                <div className="mainHeaderRight">
                  {view === "chat" && activeChatID && (
                    <button
                      type="button"
                      className="btn btnSecondary btnSmall"
                      onClick={() => void onDeleteChat()}
                      disabled={isBusy || isStreamingMessage}
                    >
                      Удалить чат
                    </button>
                  )}
                </div>
              </header>

              {view === "chat" && (
                <div className="chatPane">
                  <div className="chatScroll" ref={scrollRef}>
                    {messages.length === 0 && (
                      <div className="chatEmpty">
                        <div className="chatEmptyTitle">Задайте вопрос</div>
                        <div className="chatEmptyBody">
                          Ассистент отвечает, используя базу знаний вашей организации.
                        </div>
                      </div>
                    )}

                    {messages.map((entry) => (
                      <div
                        key={entry.id}
                        className={`chatMessageRow ${entry.role === "user" ? "fromUser" : "fromAssistant"}`}
                      >
                        <div className="chatBubble">
                          <div className="chatBubbleMeta">
                            <span className="chatRole">{entry.role === "user" ? "Вы" : "Ассистент"}</span>
                            <span className="chatMode">{entry.mode}</span>
                          </div>
                          <div className="chatBubbleContent">{entry.content}</div>
                          {entry.role === "assistant" && entry.citations?.length > 0 && (
                            <details className="details">
                              <summary className="detailsSummary">Источники</summary>
                              <div className="detailsBody">
                                <div className="sources">
                                  {entry.citations.map((citation, index) => (
                                    <button
                                      type="button"
                                      className="source sourceBtn"
                                      key={`${citation.chunk_id}-${index}`}
                                      onClick={() => onOpenCitationPreview(citation)}
                                    >
                                      <div className="sourceTitle">
                                        {citation.doc_title}{" "}
                                        <span className="sourceMeta">({citation.doc_filename})</span>
                                      </div>
                                      <div className="sourceSnippet">{citation.snippet}</div>
                                    </button>
                                  ))}
                                </div>
                              </div>
                            </details>
                          )}
                        </div>
                      </div>
                    ))}

                    {isStreamingMessage && (
                      <div className="chatMessageRow fromAssistant">
                        <div className="chatBubble">
                          <div className="chatBubbleMeta">
                            <span className="chatRole">Ассистент</span>
                            <span className="chatMode">{messageMode}</span>
                          </div>
                          <div className="chatBubbleContent">
                            {streamingAssistant || (
                              <span className="thinkingText">
                                Думает<span className="thinkingDots" aria-hidden="true"></span>
                              </span>
                            )}
                          </div>
                        </div>
                      </div>
                    )}
                  </div>

                  <div className="composerCloud" aria-hidden="true" />
                  <form className="composer" onSubmit={(event) => void onSendMessage(event)}>
                    <div className="composerCard">
                      <textarea
                        ref={composerRef}
                        value={draftMessage}
                        onChange={(event) => {
                          setDraftMessage(event.target.value);
                          event.currentTarget.style.height = "auto";
                          event.currentTarget.style.height = `${event.currentTarget.scrollHeight}px`;
                        }}
                        placeholder="Напишите сообщение…"
                        rows={1}
                        className="composerInput"
                        onKeyDown={(event) => {
                          if (event.key === "Enter" && !event.shiftKey) {
                            event.preventDefault();
                            void onSendMessage();
                          }
                        }}
                      />
                      <div className="composerBottomRow">
                        <div className="composerTabs" aria-label="Параметры сообщения">
                          <button
                            type="button"
                            className="iconCreateBtn composerActionBtn"
                            onClick={() => setIsUploadModalOpen(true)}
                            aria-label="Загрузить файл"
                          >
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="12" y1="5" x2="12" y2="19"></line><line x1="5" y1="12" x2="19" y2="12"></line></svg>
                          </button>
                          <div className="modeToggle modeToggleCompact" role="tablist" aria-label="Режим ответа">
                            <button
                              type="button"
                              className={`modeToggleItem ${messageMode === "strict" ? "active" : ""}`}
                              onClick={() => setMessageMode("strict")}
                            >
                              Строгий
                            </button>
                            <button
                              type="button"
                              className={`modeToggleItem ${messageMode === "unstrict" ? "active" : ""}`}
                              onClick={() => setMessageMode("unstrict")}
                              disabled={!canToggleWebSearch}
                              title={!canToggleWebSearch ? "Ваша роль не может использовать нестрогий режим." : undefined}
                            >
                              Нестрогий
                            </button>
                          </div>
                        </div>

                        <button
                          type="submit"
                          className="btn btnPrimary composerSendBtn"
                          disabled={isBusy || isStreamingMessage || draftMessage.trim() === ""}
                        >
                          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="22" y1="2" x2="11" y2="13"></line><polygon points="22 2 15 22 11 13 2 9 22 2"></polygon></svg>
                        </button>
                      </div>
                    </div>
                  </form>
                </div>
              )}

              {view === "knowledge" && (
                <div className="panel">
                  {!canUploadDocs && (
                    <p className="notice">Вы можете просматривать документы, но у вас нет прав на загрузку.</p>
                  )}

                  {canUploadDocs && (
                    <div className="panelSection">
                      <h2>Загрузка</h2>
                      <form className="formGrid" onSubmit={onUploadDocument}>
                        <label>
                          Название (необязательно)
                          <input
                            value={documentTitle}
                            onChange={(event) => setDocumentTitle(event.target.value)}
                            placeholder="Регламент компании"
                          />
                        </label>

                        <label>
                          Файл
                          <input
                            type="file"
                            onChange={(event) => setSelectedFile(event.target.files?.[0] || null)}
                            required
                          />
                        </label>

                        <fieldset>
                          <legend>Разрешённые роли</legend>
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

                        <button type="submit" className="btn btnPrimary" disabled={isBusy}>
                          {isBusy ? "Загрузка..." : "Загрузить документ"}
                        </button>
                      </form>
                    </div>
                  )}

                  <div className="panelSection">
                    <h2>Документы</h2>
                    {canUploadDocs && (
                      <div className="panelRow">
                        <button type="button" className="btn btnSmall" onClick={() => void onReingestAllDocuments()} disabled={isBusy}>
                          Переиндексировать все
                        </button>
                      </div>
                    )}
                    <div className="tableWrap">
                      <table>
                        <thead>
                          <tr>
                            <th>Название</th>
                            <th>Файл</th>
                            <th>Статус</th>
                            <th>Роли доступа</th>
                            {canUploadDocs && <th>Действия</th>}
                          </tr>
                        </thead>
                        <tbody>
                          {documents.map((entry) => (
                            <tr key={entry.id}>
                              <td>{entry.title}</td>
                              <td>{entry.filename}</td>
                              <td>{entry.status}</td>
                              <td>{entry.allowed_role_ids.join(", ")}</td>
                              {canUploadDocs && (
                                <td>
                                  <button
                                    type="button"
                                    className="btn btnSmall"
                                    onClick={() => void onReingestDocument(entry.id)}
                                    disabled={isBusy}
                                  >
                                    Переиндексировать
                                  </button>
                                </td>
                              )}
                            </tr>
                          ))}
                          {documents.length === 0 && (
                            <tr>
                              <td colSpan={canUploadDocs ? 5 : 4}>Документов пока нет.</td>
                            </tr>
                          )}
                        </tbody>
                      </table>
                    </div>
                  </div>
                </div>
              )}

              {view === "users" && (
                <div className="panel">
                  {!canManageUsers && <p className="notice">У текущего пользователя нет прав на управление пользователями.</p>}

                  {canManageUsers && (
                    <div className="panelSection">
                      <h2>Пользователи и роли</h2>
                      <div className="tableWrap">
                        <table>
                          <thead>
                            <tr>
                              <th>Почта</th>
                              <th>Роль</th>
                              <th>Статус</th>
                              <th>Назначить</th>
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
                                      className="btn btnSmall"
                                      onClick={() => void onChangeRole(entry.id)}
                                      disabled={isBusy}
                                    >
                                      Сохранить
                                    </button>
                                  </div>
                                </td>
                              </tr>
                            ))}
                            {users.length === 0 && (
                              <tr>
                                <td colSpan={4}>Пользователи не найдены.</td>
                              </tr>
                            )}
                          </tbody>
                        </table>
                      </div>
                    </div>
                  )}

                  {!canManageRoles && (
                    <p className="notice">У текущего пользователя нет прав на управление ролями.</p>
                  )}
                  {canManageRoles && (
                    <div className="panelSection">
                      <h2>Роли и разрешения</h2>
                      <div className="formGrid">
                        <label>
                          Название роли
                          <input
                            type="text"
                            value={roleDraftName}
                            onChange={(event) => setRoleDraftName(event.target.value)}
                            placeholder="Например: Analyst"
                            disabled={isBusy}
                          />
                        </label>
                        <div>
                          <p className="hint">Разрешения</p>
                          <div className="rolesGrid">
                            {rolePermissionOptions.map((permission) => (
                              <label key={permission.key} className="roleCheck">
                                <input
                                  type="checkbox"
                                  checked={roleDraftPermissions.includes(permission.key)}
                                  onChange={() => toggleRoleDraftPermission(permission.key)}
                                  disabled={isBusy}
                                />
                                <span>{permission.label}</span>
                              </label>
                            ))}
                          </div>
                        </div>
                        <div className="assignRow">
                          <button type="button" className="btn btnSmall" onClick={() => void onSaveRole()} disabled={isBusy}>
                            {editingRoleID ? "Обновить роль" : "Создать роль"}
                          </button>
                          {editingRoleID && (
                            <button type="button" className="btn btnGhost btnSmall" onClick={resetRoleDraft} disabled={isBusy}>
                              Отмена
                            </button>
                          )}
                        </div>
                      </div>
                      <div className="tableWrap">
                        <table>
                          <thead>
                            <tr>
                              <th>Роль</th>
                              <th>Разрешения</th>
                              <th>Действия</th>
                            </tr>
                          </thead>
                          <tbody>
                            {roles.map((role) => (
                              <tr key={role.id}>
                                <td>
                                  {role.name}
                                  {role.is_default && " (default)"}
                                </td>
                                <td>{role.permissions.join(", ") || "—"}</td>
                                <td>
                                  <div className="assignRow">
                                    <button type="button" className="btn btnSmall" onClick={() => onStartEditRole(role)} disabled={isBusy || role.is_default}>
                                      Редактировать
                                    </button>
                                    <button type="button" className="btn btnGhost btnSmall" onClick={() => void onDeleteRole(role)} disabled={isBusy || role.is_default}>
                                      Удалить
                                    </button>
                                  </div>
                                </td>
                              </tr>
                            ))}
                            {roles.length === 0 && (
                              <tr>
                                <td colSpan={3}>Роли не найдены.</td>
                              </tr>
                            )}
                          </tbody>
                        </table>
                      </div>
                    </div>
                  )}
                </div>
              )}

              {view === "account" && (
                <div className="panel">
                  <div className="panelSection">
                    <h2>Аккаунт</h2>
                    <div className="accountGrid">
                      <p>
                        <strong>Пользователь</strong>
                        <br />
                        {user.email}
                      </p>
                      <p>
                        <strong>Роль</strong>
                        <br />
                        {user.role_name}
                      </p>
                      <p>
                        <strong>Организация</strong>
                        <br />
                        {user.org_id}
                      </p>
                    </div>
                  </div>

                  <div className="panelSection">
                    <h2>Режим по умолчанию</h2>
                    <p className="hint">Определяет режим, если вы не указываете его в сообщении.</p>
                    <div className="settingsRow">
                      <div className="modeToggle" role="tablist" aria-label="Режим по умолчанию">
                        <button
                          type="button"
                          className={`modeToggleItem ${(settings?.default_mode || "strict") === "strict" ? "active" : ""}`}
                          onClick={() => void onUpdateDefaultMode("strict")}
                          disabled={isBusy}
                        >
                          Строгий
                        </button>
                        <button
                          type="button"
                          className={`modeToggleItem ${(settings?.default_mode || "strict") === "unstrict" ? "active" : ""}`}
                          onClick={() => void onUpdateDefaultMode("unstrict")}
                          disabled={isBusy || !canToggleWebSearch}
                          title={!canToggleWebSearch ? "Ваша роль не может использовать нестрогий режим." : undefined}
                        >
                          Нестрогий
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
              )}
            </section>
          </div>
        )}

        {user && isSettingsModalPresent && (
          <div
            className={`settingsModalOverlay ${isSettingsOpen ? "modalOverlayVisible" : "modalOverlayHidden"}`}
            onClick={() => setIsSettingsOpen(false)}
          >
            <div
              className={`settingsModal settingsModalWide ${isSettingsOpen ? "modalCardVisible" : "modalCardHidden"}`}
              role="dialog"
              aria-modal="true"
              aria-label="Настройки"
              onClick={(event) => event.stopPropagation()}
            >
              <div className="settingsModalBar">
                <div className="settingsModalTitle">Настройки</div>
                <button type="button" className="iconCreateBtn" onClick={() => setIsSettingsOpen(false)} aria-label="Закрыть настройки">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>
                </button>
              </div>

              <div className="settingsModalContent settingsModalContentSplit">
                <div className="settingsModalNav settingsModalNavVertical">
                  <button
                    type="button"
                    className={`settingsModalBtn ${settingsTab === "knowledge" ? "active" : ""}`}
                    onClick={() => setSettingsTab("knowledge")}
                  >
                    База знаний
                  </button>
                  <button
                    type="button"
                    className={`settingsModalBtn ${settingsTab === "users" ? "active" : ""}`}
                    disabled={!canManageUsers}
                    onClick={() => setSettingsTab("users")}
                  >
                    Пользователи
                  </button>
                  <button
                    type="button"
                    className={`settingsModalBtn ${settingsTab === "roles" ? "active" : ""}`}
                    disabled={!canManageRoles}
                    onClick={() => setSettingsTab("roles")}
                  >
                    Роли
                  </button>
                  <button
                    type="button"
                    className={`settingsModalBtn ${settingsTab === "account" ? "active" : ""}`}
                    onClick={() => setSettingsTab("account")}
                  >
                    Аккаунт
                  </button>
                  <button
                    type="button"
                    className="settingsModalBtn settingsModalBtnDanger"
                    onClick={() => void onLogout()}
                    disabled={isBusy}
                  >
                    Выйти
                  </button>
                </div>
                <div className="settingsModalPane">
                  <div className="settingsPaneContent" key={settingsTab}>
                    {settingsTab === "knowledge" && (
                      <>
                        {canUploadDocs && (
                          <div className="settingsRow">
                            <button
                              type="button"
                              className="btn btnSmall"
                              onClick={() => void onReingestAllDocuments()}
                              disabled={isBusy}
                            >
                              Переиндексировать все
                            </button>
                          </div>
                        )}
                        <div className="tableWrap">
                          <table>
                            <thead>
                              <tr>
                                <th>Название</th>
                                <th>Файл</th>
                                <th>Статус</th>
                                <th>Роли доступа</th>
                                {canUploadDocs && <th>Действия</th>}
                              </tr>
                            </thead>
                            <tbody>
                              {documents.map((entry) => (
                                <tr key={entry.id}>
                                  <td>{entry.title}</td>
                                  <td>{entry.filename}</td>
                                  <td>{entry.status}</td>
                                  <td>{entry.allowed_role_ids.join(", ")}</td>
                                  {canUploadDocs && (
                                    <td>
                                      <button
                                        type="button"
                                        className="btn btnSmall"
                                        onClick={() => void onReingestDocument(entry.id)}
                                        disabled={isBusy}
                                      >
                                        Переиндексировать
                                      </button>
                                    </td>
                                  )}
                                </tr>
                              ))}
                              {documents.length === 0 && (
                                <tr>
                                  <td colSpan={canUploadDocs ? 5 : 4}>Документов пока нет.</td>
                                </tr>
                              )}
                            </tbody>
                          </table>
                        </div>
                      </>
                    )}
                    {settingsTab === "users" &&
                      (canManageUsers ? (
                        <div className="tableWrap">
                          <table>
                            <thead>
                              <tr>
                                <th>Почта</th>
                                <th>Роль</th>
                                <th>Статус</th>
                                <th>Назначить</th>
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
                                        className="btn btnSmall"
                                        onClick={() => void onChangeRole(entry.id)}
                                        disabled={isBusy}
                                      >
                                        Сохранить
                                      </button>
                                    </div>
                                  </td>
                                </tr>
                              ))}
                              {users.length === 0 && (
                                <tr>
                                  <td colSpan={4}>Пользователи не найдены.</td>
                                </tr>
                              )}
                            </tbody>
                          </table>
                        </div>
                      ) : (
                        <p className="notice">У текущего пользователя нет прав на управление пользователями.</p>
                      ))}
                    {settingsTab === "roles" &&
                      (canManageRoles ? (
                        <>
                          <div className="formGrid">
                            <label>
                              Название роли
                              <input
                                type="text"
                                value={roleDraftName}
                                onChange={(event) => setRoleDraftName(event.target.value)}
                                placeholder="Например: Analyst"
                                disabled={isBusy}
                              />
                            </label>
                            <div>
                              <p className="hint">Разрешения</p>
                              <div className="rolesGrid">
                                {rolePermissionOptions.map((permission) => (
                                  <label key={permission.key} className="roleCheck">
                                    <input
                                      type="checkbox"
                                      checked={roleDraftPermissions.includes(permission.key)}
                                      onChange={() => toggleRoleDraftPermission(permission.key)}
                                      disabled={isBusy}
                                    />
                                    <span>{permission.label}</span>
                                  </label>
                                ))}
                              </div>
                            </div>
                            <div className="assignRow">
                              <button type="button" className="btn btnSmall" onClick={() => void onSaveRole()} disabled={isBusy}>
                                {editingRoleID ? "Обновить роль" : "Создать роль"}
                              </button>
                              {editingRoleID && (
                                <button type="button" className="btn btnGhost btnSmall" onClick={resetRoleDraft} disabled={isBusy}>
                                  Отмена
                                </button>
                              )}
                            </div>
                          </div>

                          <div className="tableWrap">
                            <table>
                              <thead>
                                <tr>
                                  <th>Роль</th>
                                  <th>Разрешения</th>
                                  <th>Действия</th>
                                </tr>
                              </thead>
                              <tbody>
                                {roles.map((role) => (
                                  <tr key={role.id}>
                                    <td>
                                      {role.name}
                                      {role.is_default && " (default)"}
                                    </td>
                                    <td>{role.permissions.join(", ") || "—"}</td>
                                    <td>
                                      <div className="assignRow">
                                        <button
                                          type="button"
                                          className="btn btnSmall"
                                          onClick={() => onStartEditRole(role)}
                                          disabled={isBusy || role.is_default}
                                        >
                                          Редактировать
                                        </button>
                                        <button
                                          type="button"
                                          className="btn btnGhost btnSmall"
                                          onClick={() => void onDeleteRole(role)}
                                          disabled={isBusy || role.is_default}
                                        >
                                          Удалить
                                        </button>
                                      </div>
                                    </td>
                                  </tr>
                                ))}
                                {roles.length === 0 && (
                                  <tr>
                                    <td colSpan={3}>Роли не найдены.</td>
                                  </tr>
                                )}
                              </tbody>
                            </table>
                          </div>
                        </>
                      ) : (
                        <p className="notice">У текущего пользователя нет прав на управление ролями.</p>
                      ))}
                    {settingsTab === "account" && (
                      <>
                        <div className="panelSection">
                          <h2>Аккаунт</h2>
                          <div className="accountGrid">
                            <p>
                              <strong>Пользователь</strong>
                              <br />
                              {user.email}
                            </p>
                            <p>
                              <strong>Роль</strong>
                              <br />
                              {user.role_name}
                            </p>
                            <p>
                              <strong>Организация</strong>
                              <br />
                              {user.org_id}
                            </p>
                          </div>
                        </div>
                        <div className="panelSection">
                          <h2>Режим по умолчанию</h2>
                          <p className="hint">Определяет режим, если вы не указываете его в сообщении.</p>
                          <div className="settingsRow">
                            <div className="modeToggle" role="tablist" aria-label="Режим по умолчанию">
                              <button
                                type="button"
                                className={`modeToggleItem ${(settings?.default_mode || "strict") === "strict" ? "active" : ""}`}
                                onClick={() => void onUpdateDefaultMode("strict")}
                                disabled={isBusy}
                              >
                                Строгий
                              </button>
                              <button
                                type="button"
                                className={`modeToggleItem ${(settings?.default_mode || "strict") === "unstrict" ? "active" : ""}`}
                                onClick={() => void onUpdateDefaultMode("unstrict")}
                                disabled={isBusy || !canToggleWebSearch}
                                title={!canToggleWebSearch ? "Ваша роль не может использовать нестрогий режим." : undefined}
                              >
                                Нестрогий
                              </button>
                            </div>
                          </div>
                        </div>
                      </>
                    )}
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}

        {user && isUploadModalPresent && (
          <div
            className={`settingsModalOverlay ${isUploadModalOpen ? "modalOverlayVisible" : "modalOverlayHidden"}`}
            onClick={() => setIsUploadModalOpen(false)}
          >
            <div
              className={`settingsModal settingsModalUpload ${isUploadModalOpen ? "modalCardVisible" : "modalCardHidden"}`}
              role="dialog"
              aria-modal="true"
              aria-label="Загрузка документа"
              onClick={(event) => event.stopPropagation()}
            >
              <div className="settingsModalBar">
                <div className="settingsModalTitle">Загрузка документа</div>
                <button type="button" className="iconCreateBtn" onClick={() => setIsUploadModalOpen(false)} aria-label="Закрыть окно загрузки">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>
                </button>
              </div>
              <div className="settingsModalContent settingsModalContentUpload">
                {canUploadDocs ? (
                  <form className="formGrid uploadFormCompact" onSubmit={onUploadDocument}>
                    <label className="uploadField">
                      Название (необязательно)
                      <input
                        value={documentTitle}
                        onChange={(event) => setDocumentTitle(event.target.value)}
                        placeholder="Регламент компании"
                      />
                    </label>
                    <label className="uploadField">
                      Файл
                      <input
                        type="file"
                        onChange={(event) => setSelectedFile(event.target.files?.[0] || null)}
                        required
                      />
                    </label>
                    <fieldset className="uploadRolesFieldset">
                      <legend>Разрешённые роли</legend>
                      <div className="rolesGrid uploadRolesGrid">
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
                    <button type="submit" className="btn btnPrimary" disabled={isBusy}>
                      {isBusy ? "Загрузка..." : "Загрузить документ"}
                    </button>
                  </form>
                ) : (
                  <p className="notice">У текущего пользователя нет прав на загрузку.</p>
                )}
              </div>
            </div>
          </div>
        )}

        {user && isCitationPreviewPresent && activeCitation && (
          <div
            className={`settingsModalOverlay ${isCitationPreviewOpen ? "modalOverlayVisible" : "modalOverlayHidden"}`}
            onClick={closeCitationPreview}
          >
            <div
              className={`settingsModal settingsModalUpload ${isCitationPreviewOpen ? "modalCardVisible" : "modalCardHidden"}`}
              role="dialog"
              aria-modal="true"
              aria-label="Просмотр источника"
              onClick={(event) => event.stopPropagation()}
            >
              <div className="settingsModalBar">
                <div className="settingsModalTitle">Источник ответа</div>
                <button type="button" className="iconCreateBtn" onClick={closeCitationPreview} aria-label="Закрыть окно источника">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>
                </button>
              </div>
              <div className="settingsModalContent settingsModalContentUpload">
                <div className="sourcePreviewGrid">
                  <p><strong>Документ</strong><br />{activeCitation.doc_title}</p>
                  <p><strong>Файл</strong><br />{activeCitation.doc_filename}</p>
                  <p><strong>Chunk ID</strong><br />{activeCitation.chunk_id}</p>
                  <p><strong>Document ID</strong><br />{activeCitation.document_id}</p>
                  <p><strong>Страница</strong><br />{activeCitation.page ?? "—"}</p>
                  <p><strong>Секция</strong><br />{activeCitation.section || "—"}</p>
                </div>
                <div className="sourcePreviewSnippet">
                  <strong>Фрагмент</strong>
                  <p>{activeCitation.snippet || "Фрагмент не указан"}</p>
                </div>
                <div className="sourcePreviewGrid">
                  <p><strong>Скоринг</strong><br />total: {activeCitation.score?.toFixed(4) ?? "—"}</p>
                  <p><strong>Vector</strong><br />{activeCitation.vector_score?.toFixed(4) ?? "—"}</p>
                  <p><strong>Text</strong><br />{activeCitation.text_score?.toFixed(4) ?? "—"}</p>
                  <p><strong>Статус документа</strong><br />{activeCitationDocument?.status || "unknown"}</p>
                </div>
                <div className="settingsRow">
                  <button
                    type="button"
                    className="btn btnSecondary btnSmall"
                    onClick={() => {
                      closeCitationPreview();
                      setSettingsTab("knowledge");
                      setIsSettingsOpen(true);
                    }}
                  >
                    Перейти к базе знаний
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}

      </section>
    </main>
  );
}

function useModalPresence(isOpen: boolean, animationMs: number) {
  const [isPresent, setIsPresent] = useState(isOpen);

  useEffect(() => {
    let timeoutID: ReturnType<typeof setTimeout> | null = null;
    if (isOpen) {
      setIsPresent(true);
    } else {
      timeoutID = setTimeout(() => setIsPresent(false), animationMs);
    }

    return () => {
      if (timeoutID) {
        clearTimeout(timeoutID);
      }
    };
  }, [animationMs, isOpen]);

  return isPresent;
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

async function streamChatMessage(
  chatID: string,
  body: Record<string, unknown>,
  token: string,
  handlers: {
    onUserMessage: (message: ChatMessage) => void;
    onAssistantDelta: (delta: string) => void;
  }
): Promise<StreamDonePayload> {
  const response = await fetch(`${APIBaseURL}/chats/${chatID}/messages/stream`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`
    },
    credentials: "include",
    body: JSON.stringify(body)
  });

  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    const reason = typeof payload?.error === "string" ? payload.error : `Request failed: ${response.status}`;
    throw new Error(reason);
  }

  if (!response.body) {
    throw new Error("Streaming response body is missing");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let donePayload: StreamDonePayload | null = null;

  while (true) {
    const chunk = await reader.read();
    if (chunk.done) {
      break;
    }

    buffer += decoder.decode(chunk.value, { stream: true });

    let separatorIndex = buffer.indexOf("\n\n");
    for (; separatorIndex !== -1; separatorIndex = buffer.indexOf("\n\n")) {
      const rawEvent = buffer.slice(0, separatorIndex);
      buffer = buffer.slice(separatorIndex + 2);

      const parsed = parseSSEEvent(rawEvent);
      if (!parsed) {
        continue;
      }

      if (parsed.event === "user_message") {
        const payload = JSON.parse(parsed.data) as ChatMessage;
        handlers.onUserMessage(payload);
      }
      if (parsed.event === "assistant_delta") {
        const payload = JSON.parse(parsed.data) as { delta?: string };
        if (payload.delta) {
          handlers.onAssistantDelta(payload.delta);
        }
      }
      if (parsed.event === "done") {
        donePayload = JSON.parse(parsed.data) as StreamDonePayload;
      }
    }
  }

  if (!donePayload) {
    throw new Error("Stream finished without done event");
  }

  return donePayload;
}

function parseSSEEvent(rawChunk: string): { event: string; data: string } | null {
  const normalized = rawChunk.replace(/\r/g, "").trim();
  if (!normalized) {
    return null;
  }

  let event = "message";
  const dataLines: string[] = [];

  normalized.split("\n").forEach((line) => {
    if (line.startsWith("event:")) {
      event = line.slice("event:".length).trim();
      return;
    }
    if (line.startsWith("data:")) {
      let data = line.slice("data:".length);
      if (data.startsWith(" ")) {
        data = data.slice(1);
      }
      dataLines.push(data);
    }
  });

  return {
    event,
    data: dataLines.join("\n")
  };
}

function errorMessage(value: unknown): string {
  if (value instanceof Error) {
    return value.message;
  }

  return "Unexpected request error";
}
