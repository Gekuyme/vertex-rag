"use client";

import type { FormEvent } from "react";
import { useEffect, useMemo, useRef, useState } from "react";
import { getMessages } from "../../lib/i18n/messages";
import { useI18n } from "./I18nProvider";
import LocaleSwitcher from "./LocaleSwitcher";

const APIBaseURL = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080";
const accessTokenStorageKey = "vertex_access_token";
const noKnowledgeMessages: string[] = [getMessages("ru").chat.noKnowledge, getMessages("en").chat.noKnowledge];

type ConsoleView = "chat" | "knowledge" | "users" | "account";
type SettingsTab = "knowledge" | "users" | "roles" | "account";
type OnboardingStep = {
  selector: string;
};

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
  client_status?: "pending" | "failed";
};

type RequestOptions = {
  method?: "GET" | "POST" | "PATCH" | "DELETE";
  body?: unknown;
  token?: string;
  signal?: AbortSignal;
  requestFailureMessage?: (status: number) => string;
};

type StreamDonePayload = {
  mode: "strict" | "unstrict";
  user_message: ChatMessage;
  assistant_message: ChatMessage;
  citations: Citation[];
};

type ResponseProfile = "fast" | "balanced" | "thinking";
type StreamPhase = "retrieving" | "drafting" | "finalizing";
type ComposerDropdown = "mode" | "profile" | null;

type ConsoleAppProps = {
  initialView?: ConsoleView;
};

const onboardingSeenStorageKey = "vertex_onboarding_seen_v1";
const responseProfilePresets: Array<{
  id: ResponseProfile;
  topK: number;
  candidateK: number;
}> = [
  { id: "fast", topK: 4, candidateK: 12 },
  { id: "balanced", topK: 8, candidateK: 32 },
  { id: "thinking", topK: 12, candidateK: 48 }
];
const onboardingStepTargets: OnboardingStep[] = [
  { selector: '[data-tour="chat-nav"]' },
  { selector: '[data-tour="chat-create"]' },
  { selector: '[data-tour="settings-btn"]' },
  { selector: '[data-tour="upload-btn"]' },
  { selector: '[data-tour="mode-toggle"]' }
];

export default function ConsoleApp({ initialView = "chat" }: ConsoleAppProps) {
  const { messages } = useI18n();
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
  const [chatMessages, setChatMessages] = useState<ChatMessage[]>([]);
  const [draftMessage, setDraftMessage] = useState("");
  const [messageMode, setMessageMode] = useState<"strict" | "unstrict">("strict");
  const [responseProfile, setResponseProfile] = useState<ResponseProfile>("balanced");
  const [openComposerDropdown, setOpenComposerDropdown] = useState<ComposerDropdown>(null);
  const [streamingAssistant, setStreamingAssistant] = useState("");
  const [streamPhase, setStreamPhase] = useState<StreamPhase | null>(null);
  const [streamStartedAt, setStreamStartedAt] = useState<number | null>(null);
  const [streamElapsedSeconds, setStreamElapsedSeconds] = useState(0);
  const [assistantResponseDurations, setAssistantResponseDurations] = useState<Record<string, number>>({});
  const [isStreamingMessage, setIsStreamingMessage] = useState(false);
  const [isCitationPreviewOpen, setIsCitationPreviewOpen] = useState(false);
  const [activeCitation, setActiveCitation] = useState<Citation | null>(null);
  const [isOnboardingOpen, setIsOnboardingOpen] = useState(false);
  const [onboardingStepIndex, setOnboardingStepIndex] = useState(0);
  const [onboardingRect, setOnboardingRect] = useState<DOMRect | null>(null);
  const [onboardingCardPosition, setOnboardingCardPosition] = useState<{ top: number; left: number } | null>(null);

  const composerRef = useRef<HTMLTextAreaElement | null>(null);
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const chatLoadAbortRef = useRef<AbortController | null>(null);
  const chatStreamAbortRef = useRef<AbortController | null>(null);
  const streamingAssistantBufferRef = useRef("");
  const streamingAssistantFlushFrameRef = useRef<number | null>(null);

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
  const canUseUnstrict = useMemo(
    () => Boolean(user?.permissions?.includes("can_use_unstrict")),
    [user]
  );
  const canAccessUnstrict = canUseUnstrict || canToggleWebSearch;
  const rolePermissionOptions = useMemo(
    () => [
      { key: "can_upload_docs", label: messages.roles.permissionUploadDocs },
      { key: "can_manage_users", label: messages.roles.permissionManageUsers },
      { key: "can_manage_roles", label: messages.roles.permissionManageRoles },
      { key: "can_manage_documents", label: messages.roles.permissionManageDocuments },
      { key: "can_toggle_web_search", label: messages.roles.permissionToggleWebSearch },
      { key: "can_use_unstrict", label: messages.roles.permissionUseUnstrict }
    ],
    [messages]
  );
  const responseProfiles = useMemo(
    () =>
      responseProfilePresets.map((profile) => ({
        ...profile,
        label: messages.responseProfiles[profile.id]
      })),
    [messages]
  );
  const onboardingSteps = useMemo(
    () =>
      onboardingStepTargets.map((step, index) => ({
        ...step,
        ...messages.onboarding.steps[index]
      })),
    [messages]
  );
  const isUploadReady = Boolean(selectedFile) && selectedRoleIDs.length > 0 && !isBusy;
  const uploadHint = !selectedFile
    ? messages.uploadModal.selectFile
    : selectedRoleIDs.length === 0
      ? messages.uploadModal.selectRole
      : "";
  const isSettingsModalPresent = useModalPresence(isSettingsOpen, modalAnimationMs);
  const isUploadModalPresent = useModalPresence(isUploadModalOpen, modalAnimationMs);
  const isCitationPreviewPresent = useModalPresence(isCitationPreviewOpen, modalAnimationMs);
  const currentOnboardingStep = onboardingSteps[onboardingStepIndex] || null;
  const activeResponseProfile = responseProfiles.find((profile) => profile.id === responseProfile) || responseProfiles[1];
  const currentRequestFailureMessage = messages.feedback.requestFailed;

  const request = <T,>(path: string, options: RequestOptions = {}) =>
    apiRequest<T>(path, {
      ...options,
      requestFailureMessage: options.requestFailureMessage || currentRequestFailureMessage
    });

  const requestMultipart = <T,>(path: string, formData: FormData, accessToken: string) =>
    apiRequestMultipart<T>(path, formData, accessToken, currentRequestFailureMessage);

  const requestStream = (
    chatID: string,
    body: Record<string, unknown>,
    accessToken: string,
    handlers: {
      signal?: AbortSignal;
      onPhase: (phase: StreamPhase) => void;
      onUserMessage: (message: ChatMessage) => void;
      onAssistantDelta: (delta: string) => void;
    }
  ) =>
    streamChatMessage(chatID, body, accessToken, {
      ...handlers,
      requestFailureMessage: currentRequestFailureMessage
    });

  useEffect(() => {
    const storedToken = window.localStorage.getItem(accessTokenStorageKey);
    if (!storedToken) {
      return;
    }

    setToken(storedToken);
    void hydrateSession(storedToken);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => () => {
    chatLoadAbortRef.current?.abort();
    chatStreamAbortRef.current?.abort();
    if (streamingAssistantFlushFrameRef.current !== null) {
      window.cancelAnimationFrame(streamingAssistantFlushFrameRef.current);
      streamingAssistantFlushFrameRef.current = null;
    }
    streamingAssistantBufferRef.current = "";
  }, []);

  useEffect(() => {
    if (!user) {
      return;
    }

    // Sync chat mode selector with user default.
    if (settings?.default_mode) {
      if (settings.default_mode === "unstrict" && !canAccessUnstrict) {
        setMessageMode("strict");
        return;
      }
      setMessageMode(settings.default_mode);
    } else {
      setMessageMode("strict");
    }
  }, [canAccessUnstrict, settings?.default_mode, user]);

  useEffect(() => {
    if (messageMode === "unstrict" && !canAccessUnstrict) {
      setMessageMode("strict");
    }
  }, [canAccessUnstrict, messageMode]);

  useEffect(() => {
    function onPointerDown(event: PointerEvent) {
      const target = event.target;
      if (!(target instanceof Element)) {
        return;
      }
      if (target.closest("[data-composer-dropdown]")) {
        return;
      }
      setOpenComposerDropdown(null);
    }

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setOpenComposerDropdown(null);
      }
    }

    window.addEventListener("pointerdown", onPointerDown);
    window.addEventListener("keydown", onKeyDown);
    return () => {
      window.removeEventListener("pointerdown", onPointerDown);
      window.removeEventListener("keydown", onKeyDown);
    };
  }, []);

  useEffect(() => {
    if (!isStreamingMessage || streamStartedAt === null) {
      return;
    }

    const updateElapsed = () => {
      setStreamElapsedSeconds(Math.max(0, Math.floor((Date.now() - streamStartedAt) / 1000)));
    };

    updateElapsed();
    const intervalID = window.setInterval(updateElapsed, 1000);
    return () => window.clearInterval(intervalID);
  }, [isStreamingMessage, streamStartedAt]);

  function flushStreamingAssistantBuffer() {
    if (streamingAssistantFlushFrameRef.current !== null) {
      window.cancelAnimationFrame(streamingAssistantFlushFrameRef.current);
      streamingAssistantFlushFrameRef.current = null;
    }

    const bufferedDelta = streamingAssistantBufferRef.current;
    if (!bufferedDelta) {
      return;
    }

    streamingAssistantBufferRef.current = "";
    setStreamingAssistant((current) => current + bufferedDelta);
  }

  function queueStreamingAssistantDelta(delta: string) {
    if (!delta) {
      return;
    }

    streamingAssistantBufferRef.current += delta;
    if (streamingAssistantFlushFrameRef.current !== null) {
      return;
    }

    streamingAssistantFlushFrameRef.current = window.requestAnimationFrame(() => {
      streamingAssistantFlushFrameRef.current = null;
      flushStreamingAssistantBuffer();
    });
  }

  function resetStreamingAssistant() {
    if (streamingAssistantFlushFrameRef.current !== null) {
      window.cancelAnimationFrame(streamingAssistantFlushFrameRef.current);
      streamingAssistantFlushFrameRef.current = null;
    }
    streamingAssistantBufferRef.current = "";
    setStreamingAssistant("");
    setStreamPhase(null);
  }

  function streamPhaseLabel(phase: StreamPhase | null, profile: ResponseProfile): string {
    switch (phase) {
      case "retrieving":
        return profile === "fast" ? messages.chat.streamRetrievingFast : messages.chat.streamRetrieving;
      case "drafting":
        return profile === "thinking" ? messages.chat.streamDraftingThinking : messages.chat.streamDrafting;
      case "finalizing":
        return messages.chat.streamFinalizing;
      default:
        return profile === "thinking" ? messages.chat.streamThinkingDeep : messages.chat.streamThinking;
    }
  }

  function formatElapsed(totalSeconds: number): string {
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    return `${minutes}:${String(seconds).padStart(2, "0")}`;
  }

  const modeOptions: Array<{ value: "strict" | "unstrict"; label: string }> = canAccessUnstrict
    ? [
        { value: "strict", label: messages.chat.modeStrict },
        { value: "unstrict", label: messages.chat.modeUnstrict }
      ]
    : [{ value: "strict", label: messages.chat.modeStrict }];

  function getTranslatedStatus(status: string) {
    const normalized = status.toLowerCase() as keyof typeof messages.status;
    return messages.status[normalized] || status;
  }

  function getTranslatedPermission(permissionKey: string) {
    return rolePermissionOptions.find((permission) => permission.key === permissionKey)?.label || permissionKey;
  }

  function formatPermissionList(permissionKeys: string[]) {
    if (permissionKeys.length === 0) {
      return "—";
    }

    return permissionKeys.map(getTranslatedPermission).join(", ");
  }

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

  useEffect(() => {
    if (!user || view !== "chat") {
      return;
    }
    const hasSeenOnboarding = window.localStorage.getItem(onboardingSeenStorageKey) === "1";
    if (!hasSeenOnboarding) {
      setOnboardingStepIndex(0);
      setIsOnboardingOpen(true);
    }
  }, [user, view]);

  useEffect(() => {
    if (!isOnboardingOpen || !currentOnboardingStep) {
      return;
    }
    let activeTarget: HTMLElement | null = null;

    function refreshOnboardingRect() {
      const target = document.querySelector(currentOnboardingStep.selector) as HTMLElement | null;
      if (!target) {
        activeTarget?.classList.remove("onboardingTargetActive");
        activeTarget = null;
        setOnboardingRect(null);
        setOnboardingCardPosition(null);
        return;
      }
      if (activeTarget !== target) {
        activeTarget?.classList.remove("onboardingTargetActive");
        activeTarget = target;
        activeTarget.classList.add("onboardingTargetActive");
      }
      const rect = target.getBoundingClientRect();
      setOnboardingRect(rect);
      const margin = 12;
      const cardWidth = Math.min(360, window.innerWidth - margin * 2);
      const estimatedCardHeight = 230;
      const maxTop = Math.max(margin, window.innerHeight - estimatedCardHeight - margin);
      let top = rect.bottom + 14;
      if (top > maxTop) {
        top = rect.top - estimatedCardHeight - 14;
      }
      top = Math.max(margin, Math.min(top, maxTop));
      const preferredLeft = rect.left + rect.width / 2 - cardWidth / 2;
      const maxLeft = Math.max(margin, window.innerWidth - cardWidth - margin);
      const left = Math.max(margin, Math.min(preferredLeft, maxLeft));
      setOnboardingCardPosition({ top, left });
    }

    refreshOnboardingRect();
    const target = document.querySelector(currentOnboardingStep.selector) as HTMLElement | null;
    target?.scrollIntoView({ behavior: "smooth", block: "center", inline: "center" });

    window.addEventListener("resize", refreshOnboardingRect);
    window.addEventListener("scroll", refreshOnboardingRect, true);
    return () => {
      activeTarget?.classList.remove("onboardingTargetActive");
      window.removeEventListener("resize", refreshOnboardingRect);
      window.removeEventListener("scroll", refreshOnboardingRect, true);
    };
  }, [currentOnboardingStep, isOnboardingOpen]);

  async function hydrateSession(accessToken: string) {
    try {
      const profile = await request<User>("/me", { token: accessToken });
      setUser(profile);

      const [nextSettings] = await Promise.all([
        request<UserSettings>("/me/settings", { token: accessToken }),
        loadWorkspace(accessToken, profile),
        bootstrapChats(accessToken)
      ]);
      setSettings(nextSettings);

      setMessage(messages.feedback.signedIn(profile.email));
      setError("");
      return;
    } catch {
      try {
        const refreshed = await request<AuthResponse>("/auth/refresh", { method: "POST" });
        persistAccessToken(refreshed.access_token);
        setUser(refreshed.user);

        const [nextSettings] = await Promise.all([
          request<UserSettings>("/me/settings", { token: refreshed.access_token }),
          loadWorkspace(refreshed.access_token, refreshed.user),
          bootstrapChats(refreshed.access_token)
        ]);
        setSettings(nextSettings);

        setMessage(messages.feedback.sessionRestored(refreshed.user.email));
        setError("");
      } catch {
        clearSession();
      }
    }
  }

  async function loadWorkspace(accessToken: string, profile: User) {
    const [roleResponse, documentResponse, userResponse] = await Promise.all([
      request<{ roles: Role[] }>("/roles", { token: accessToken }),
      request<{ documents: DocumentEntry[] }>("/documents", { token: accessToken }),
      profile.permissions.includes("can_manage_users")
        ? request<{ users: User[] }>("/admin/users", { token: accessToken })
        : Promise.resolve(null)
    ]);

    setRoles(roleResponse.roles);
    if (roleResponse.roles.length > 0) {
      setSelectedRoleIDs([profile.role_id]);
    }

    setDocuments(documentResponse.documents);

    if (userResponse) {
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
    const chatResponse = await request<{ chats: Chat[] }>("/chats", { token: accessToken });
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

    const created = await request<Chat>("/chats", { method: "POST", token: accessToken, body: { title: "" } });
    setChats([created]);
    setActiveChatID(created.id);
    setChatMessages([]);
  }

  async function loadChatMessages(accessToken: string, chatID: string) {
    chatLoadAbortRef.current?.abort();
    const controller = new AbortController();
    chatLoadAbortRef.current = controller;

    try {
      const response = await request<{ chat: Chat; messages: ChatMessage[] }>(`/chats/${chatID}/messages`, {
        token: accessToken,
        signal: controller.signal
      });
      if (chatLoadAbortRef.current === controller) {
        setChatMessages(response.messages);
      }
    } catch (requestError) {
      if (!isAbortError(requestError)) {
        throw requestError;
      }
    } finally {
      if (chatLoadAbortRef.current === controller) {
        chatLoadAbortRef.current = null;
      }
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
        authResponse = await request<AuthResponse>("/auth/register_owner", {
          method: "POST",
          body: {
            organization_name: organizationName,
            email,
            password
          }
        });
      } else {
        authResponse = await request<AuthResponse>("/auth/login", {
          method: "POST",
          body: { email, password }
        });
      }

      persistAccessToken(authResponse.access_token);
      setUser(authResponse.user);

      const [nextSettings] = await Promise.all([
        request<UserSettings>("/me/settings", { token: authResponse.access_token }),
        loadWorkspace(authResponse.access_token, authResponse.user),
        bootstrapChats(authResponse.access_token)
      ]);
      setSettings(nextSettings);

      setMessage(
        mode === "register"
          ? messages.feedback.organizationCreated(authResponse.user.email)
          : messages.feedback.signedIn(authResponse.user.email)
      );
    } catch (requestError) {
      setError(errorMessage(requestError, messages.feedback.unexpectedError));
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
      await request(`/admin/users/${userID}/role`, {
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
      setMessage(messages.feedback.userRoleUpdated);
    } catch (requestError) {
      setError(errorMessage(requestError, messages.feedback.unexpectedError));
    } finally {
      setIsBusy(false);
    }
  }

  async function onUploadDocument(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedFile) {
      setError(messages.uploadModal.selectFile);
      return;
    }
    if (selectedRoleIDs.length === 0) {
      setError(messages.uploadModal.selectRole);
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

      await requestMultipart<DocumentEntry>("/documents/upload", formData, token);
      if (user) {
        await loadWorkspace(token, user);
      }

      setSelectedFile(null);
      setDocumentTitle("");
      setSelectedRoleIDs([]);
      setIsUploadModalOpen(false);
      setMessage(messages.feedback.documentUploaded);
    } catch (requestError) {
      setError(errorMessage(requestError, messages.feedback.unexpectedError));
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
      await request(`/documents/${documentID}/reingest`, {
        method: "POST",
        token
      });
      await loadWorkspace(token, user);
      setMessage(messages.feedback.documentReingest);
    } catch (requestError) {
      setError(errorMessage(requestError, messages.feedback.unexpectedError));
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
      const response = await request<{ scheduled_count: number; total_count: number }>("/documents/reingest_all", {
        method: "POST",
        token
      });
      await loadWorkspace(token, user);
      setMessage(messages.feedback.allDocumentsReingest(response.scheduled_count, response.total_count));
    } catch (requestError) {
      setError(errorMessage(requestError, messages.feedback.unexpectedError));
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
      const created = await request<Chat>("/chats", { method: "POST", token, body: { title: "" } });
      setChats((current) => [created, ...current]);
      setActiveChatID(created.id);
      setChatMessages([]);
      setView("chat");
      setIsNavOpen(false);
      setMessage(messages.feedback.newChatCreated);
      queueMicrotask(() => composerRef.current?.focus());
    } catch (requestError) {
      setError(errorMessage(requestError, messages.feedback.unexpectedError));
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
      setError(errorMessage(requestError, messages.feedback.unexpectedError));
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
      created_at: new Date().toISOString(),
      client_status: "pending"
    };

    setIsBusy(true);
    setIsStreamingMessage(true);
    setMessage("");
    setError("");
    setDraftMessage("");
    resetStreamingAssistant();
    setStreamPhase("retrieving");
    setStreamStartedAt(Date.now());
    setStreamElapsedSeconds(0);
    setChatMessages((current) => [...current, optimisticUserMessage]);
    let persistedUserMessageID = "";
    const localAssistantErrorID = `${optimisticMessageID}-error`;
    chatStreamAbortRef.current?.abort();
    const streamController = new AbortController();
    chatStreamAbortRef.current = streamController;

    try {
      const payload: Record<string, unknown> = {
        content
      };

      // If mode is explicitly selected, send it.
      if (messageMode) {
        payload.mode = messageMode;
      }
      payload.top_k = activeResponseProfile.topK;
      payload.candidate_k = activeResponseProfile.candidateK;

      const response = await requestStream(activeChatID, payload, token, {
        signal: streamController.signal,
        onPhase(nextPhase) {
          setStreamPhase(nextPhase);
        },
        onUserMessage(nextMessage) {
          persistedUserMessageID = nextMessage.id;
          setChatMessages((current) => {
            let didReplaceOptimistic = false;
            const replaced = current.map((entry) => {
              if (entry.id === optimisticMessageID) {
                didReplaceOptimistic = true;
                return nextMessage;
              }
              return entry;
            });
            const withoutLocalError = replaced.filter((entry) => entry.id !== localAssistantErrorID);
            if (withoutLocalError.some((entry) => entry.id === nextMessage.id)) {
              return withoutLocalError;
            }
            return didReplaceOptimistic ? withoutLocalError : [...withoutLocalError, nextMessage];
          });
        },
        onAssistantDelta(delta) {
          queueStreamingAssistantDelta(delta);
        }
      });

      setChatMessages((current) => {
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
      if (streamStartedAt !== null) {
        const elapsedSeconds = Math.max(0, Math.floor((Date.now() - streamStartedAt) / 1000));
        setAssistantResponseDurations((current) => ({
          ...current,
          [response.assistant_message.id]: elapsedSeconds
        }));
      }
      resetStreamingAssistant();
      // Refresh list so sidebar ordering stays accurate, but keep active chat and local messages.
      void refreshChats(token).catch(() => {});
    } catch (requestError) {
      resetStreamingAssistant();
      if (isAbortError(requestError)) {
        return;
      }
      const failedMessageID = typeof persistedUserMessageID === "string" && persistedUserMessageID !== ""
        ? persistedUserMessageID
        : optimisticMessageID;
      const shortReason = messages.feedback.responseMissing;
      setChatMessages((current) => {
        const localAssistantErrorID = `${optimisticMessageID}-error`;
        const nextMessages = current.map((entry) =>
          entry.id === failedMessageID
            ? {
                ...entry,
                client_status: "failed" as const
              }
            : entry
        );
        if (nextMessages.some((entry) => entry.id === localAssistantErrorID)) {
          return nextMessages;
        }
        const localAssistantErrorMessage: ChatMessage = {
          id: localAssistantErrorID,
          chat_id: activeChatID,
          role: "assistant",
          mode: messageMode,
          content: shortReason,
          citations: [],
          created_at: new Date().toISOString(),
          client_status: "failed"
        };
        return [
          ...nextMessages,
          localAssistantErrorMessage
        ];
      });
      setError(shortReason);
    } finally {
      if (chatStreamAbortRef.current === streamController) {
        chatStreamAbortRef.current = null;
      }
      setIsBusy(false);
      setIsStreamingMessage(false);
      setStreamStartedAt(null);
      queueMicrotask(() => composerRef.current?.focus());
    }
  }

  async function onDeleteChat() {
    if (!token || !activeChatID) {
      return;
    }

    if (!window.confirm(messages.feedback.deleteChatConfirm)) {
      return;
    }

    setIsBusy(true);
    setMessage("");
    setError("");
    try {
      await request(`/chats/${activeChatID}`, {
        method: "DELETE",
        token
      });

      const list = await refreshChats(token);
      if (list.length === 0) {
        const created = await request<Chat>("/chats", {
          method: "POST",
          token,
          body: { title: "" }
        });
        setChats([created]);
        setActiveChatID(created.id);
        setChatMessages([]);
      } else {
        const nextChatID = list[0].id;
        setActiveChatID(nextChatID);
        await loadChatMessages(token, nextChatID);
      }
      setMessage(messages.feedback.chatDeleted);
    } catch (requestError) {
      setError(errorMessage(requestError, messages.feedback.unexpectedError));
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
      const updated = await request<UserSettings>("/me/settings", {
        method: "PATCH",
        token,
        body: { default_mode: nextMode }
      });
      setSettings(updated);
      setMessage(messages.feedback.defaultModeUpdated);
    } catch (requestError) {
      setError(errorMessage(requestError, messages.feedback.unexpectedError));
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
      setError(messages.feedback.roleNameRequired);
      return;
    }

    setIsBusy(true);
    setMessage("");
    setError("");
    try {
      if (editingRoleID) {
        await request(`/admin/roles/${editingRoleID}`, {
          method: "PATCH",
          token,
          body: {
            name: cleanName,
            permissions: roleDraftPermissions
          }
        });
        setMessage(messages.feedback.roleUpdated);
      } else {
        await request("/admin/roles", {
          method: "POST",
          token,
          body: {
            name: cleanName,
            permissions: roleDraftPermissions
          }
        });
        setMessage(messages.feedback.roleCreated);
      }

      resetRoleDraft();
      await loadWorkspace(token, user);
    } catch (requestError) {
      setError(errorMessage(requestError, messages.feedback.unexpectedError));
    } finally {
      setIsBusy(false);
    }
  }

  async function onDeleteRole(role: Role) {
    if (!token || !user || !canManageRoles) {
      return;
    }
    if (role.is_default) {
      setError(messages.feedback.systemRoleDelete);
      return;
    }

    if (!window.confirm(messages.feedback.deleteRoleConfirm(role.name))) {
      return;
    }

    setIsBusy(true);
    setMessage("");
    setError("");
    try {
      await request(`/admin/roles/${role.id}`, {
        method: "DELETE",
        token
      });
      if (editingRoleID === role.id) {
        resetRoleDraft();
      }
      await loadWorkspace(token, user);
      setMessage(messages.feedback.roleDeleted);
    } catch (requestError) {
      setError(errorMessage(requestError, messages.feedback.unexpectedError));
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

  function closeOnboarding(markAsSeen = true) {
    setIsOnboardingOpen(false);
    if (markAsSeen) {
      window.localStorage.setItem(onboardingSeenStorageKey, "1");
    }
  }

  function onNextOnboardingStep() {
    if (onboardingStepIndex >= onboardingSteps.length - 1) {
      closeOnboarding(true);
      return;
    }
    setOnboardingStepIndex((current) => Math.min(current + 1, onboardingSteps.length - 1));
  }

  function onPrevOnboardingStep() {
    setOnboardingStepIndex((current) => Math.max(current - 1, 0));
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
      await request("/auth/logout", { method: "POST" });
      clearSession();
      setMessage(messages.feedback.loggedOut);
    } catch (requestError) {
      setError(errorMessage(requestError, messages.feedback.unexpectedError));
    } finally {
      setIsBusy(false);
    }
  }

  function persistAccessToken(nextToken: string) {
    window.localStorage.setItem(accessTokenStorageKey, nextToken);
    setToken(nextToken);
  }

  function clearSession() {
    chatLoadAbortRef.current?.abort();
    chatLoadAbortRef.current = null;
    chatStreamAbortRef.current?.abort();
    chatStreamAbortRef.current = null;
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
    setChatMessages([]);
    setAssistantResponseDurations({});
    setDraftMessage("");
    resetStreamingAssistant();
    setIsStreamingMessage(false);
    setStreamStartedAt(null);
    setStreamElapsedSeconds(0);
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
        {message && <div className="message success">{message}</div>}
        {error && <div className="message error">{error}</div>}

        {!user && (
          <div className="headerRow">
            <div className="headerRowTop">
              <div>
                <h1>{messages.auth.loginTitle}</h1>
              </div>
              <LocaleSwitcher />
            </div>
            <p className="hint">{messages.auth.ownerConsole}</p>
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
                {messages.auth.loginTab}
              </button>
              <button
                type="button"
                className={`tabButton ${mode === "register" ? "active" : ""}`}
                onClick={() => setMode("register")}
              >
                {messages.auth.registerTab}
              </button>
            </div>

            <form className="formGrid" onSubmit={onSubmitAuth}>
              {mode === "register" && (
                <label>
                  {messages.auth.organization}
                  <input
                    value={organizationName}
                    onChange={(event) => setOrganizationName(event.target.value)}
                    placeholder={messages.auth.organizationPlaceholder}
                    required
                  />
                </label>
              )}

              <label>
                {messages.auth.email}
                <input
                  type="email"
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  placeholder="owner@company.com"
                  required
                />
              </label>

              <label>
                {messages.auth.password}
                <input
                  type="password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  minLength={8}
                  required
                />
              </label>

              <button type="submit" className="btn btnPrimary" disabled={isBusy}>
                {isBusy
                  ? messages.auth.submitBusy
                  : mode === "register"
                    ? messages.auth.submitRegister
                    : messages.auth.submitLogin}
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
                  <div className="sidebarSub">{messages.shell.workspace}</div>
                </div>
                <LocaleSwitcher />
              </div>

              <div className="sidebarSection" data-tour="chat-nav">
                <div className="sidebarSectionHeader">
                  <div className="sidebarSectionTitle">{messages.shell.chats}</div>
                  <div className="iconBtnRow">
                    <button
                      type="button"
                      className="iconCreateBtn"
                      onClick={() => void onCreateChat()}
                      aria-label={messages.shell.createChat}
                      data-tour="chat-create"
                    >
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
                  {chats.length === 0 && <div className="chatListEmpty">{messages.shell.noChats}</div>}
                </div>
              </div>

              <div className="sidebarFooter">
                <button
                  type="button"
                  className="btn btnSecondary btnSmall sidebarSettingsBtn"
                  aria-label={messages.shell.openSettings}
                  data-tour="settings-btn"
                  onClick={() => {
                    setSettingsTab("account");
                    setIsSettingsOpen(true);
                  }}
                >
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="3"></circle><path d="M19.4 15A1.65 1.65 0 0 0 20 13.6a1.65 1.65 0 0 0-.6-1.2l-1.1-.9a1.65 1.65 0 0 1-.5-1.8l.5-1.3a1.65 1.65 0 0 0-.3-1.8 1.65 1.65 0 0 0-1.7-.5l-1.4.4a1.65 1.65 0 0 1-1.7-.5l-.9-1.1A1.65 1.65 0 0 0 12.4 4a1.65 1.65 0 0 0-1.2.6l-.9 1.1a1.65 1.65 0 0 1-1.8.5l-1.3-.5a1.65 1.65 0 0 0-1.8.3 1.65 1.65 0 0 0-.5 1.7l.4 1.4a1.65 1.65 0 0 1-.5 1.7L4 11.6a1.65 1.65 0 0 0-.6 1.2A1.65 1.65 0 0 0 4 14.2l1.1.9a1.65 1.65 0 0 1 .5 1.8l-.5 1.3a1.65 1.65 0 0 0 .3 1.8 1.65 1.65 0 0 0 1.7.5l1.4-.4a1.65 1.65 0 0 1 1.7.5l.9 1.1a1.65 1.65 0 0 0 1.2.6 1.65 1.65 0 0 0 1.2-.6l.9-1.1a1.65 1.65 0 0 1 1.8-.5l1.3.5a1.65 1.65 0 0 0 1.8-.3 1.65 1.65 0 0 0 .5-1.7l-.4-1.4a1.65 1.65 0 0 1 .5-1.7z"></path></svg>
                  <span>{messages.shell.settings}</span>
                </button>
              </div>
            </aside>

            <section className="mainPane">
              <header className="mainHeader">
                <button
                  type="button"
                  className="btn btnSecondary btnSmall navToggle"
                  onClick={() => setIsNavOpen((current) => !current)}
                  aria-label={messages.shell.toggleNavigation}
                >
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="3" y1="12" x2="21" y2="12"></line><line x1="3" y1="6" x2="21" y2="6"></line><line x1="3" y1="18" x2="21" y2="18"></line></svg>
                </button>

                <div className="mainHeaderTitle">
                  {view === "chat" && <span>{activeChat?.title || messages.shell.newChat}</span>}
                  {view === "knowledge" && <span>{messages.shell.knowledge}</span>}
                  {view === "users" && <span>{messages.shell.users}</span>}
                  {view === "account" && <span>{messages.shell.account}</span>}
                </div>
                <div className="mainHeaderRight">
                  {view === "chat" && activeChatID && (
                    <button
                      type="button"
                      className="btn btnSecondary btnSmall"
                      onClick={() => void onDeleteChat()}
                      disabled={isBusy || isStreamingMessage}
                    >
                      {messages.shell.deleteChat}
                    </button>
                  )}
                </div>
              </header>

              {view === "chat" && (
                <div className="chatPane">
                  <div className="chatScroll" ref={scrollRef}>
                    {chatMessages.length === 0 && (
                      <div className="chatEmpty">
                        <div className="chatEmptyTitle">{messages.chat.emptyTitle}</div>
                        <div className="chatEmptyBody">{messages.chat.emptyBody}</div>
                      </div>
                    )}

                    {chatMessages.map((entry) => (
                      <div
                        key={entry.id}
                        className={`chatMessageRow ${entry.role === "user" ? "fromUser" : "fromAssistant"} ${entry.client_status === "failed" ? "isFailed" : ""}`}
                      >
                        <div className="chatBubble">
                          <div className="chatBubbleMeta">
                            <span className="chatRole">{entry.role === "user" ? messages.chat.you : messages.chat.assistant}</span>
                            {entry.role === "assistant" && assistantResponseDurations[entry.id] !== undefined && (
                              <span className="chatMetaTime">{formatElapsed(assistantResponseDurations[entry.id])}</span>
                            )}
                          </div>
                          <div className="chatBubbleContent">{entry.content}</div>
                          {entry.role === "user" && entry.client_status === "failed" && (
                            <div className="chatBubbleHint">{messages.chat.failedDelivery}</div>
                          )}
                          {entry.role === "assistant" &&
                            entry.mode === "strict" &&
                            noKnowledgeMessages.includes(entry.content.trim()) &&
                            (entry.citations?.length || 0) === 0 && (
                              <div className="chatBubbleHint">{messages.chat.noKnowledgeHint}</div>
                            )}
                          {entry.role === "assistant" && entry.citations?.length > 0 && (
                            <details className="details">
                              <summary className="detailsSummary">{messages.chat.sources}</summary>
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
                            <span className="chatRole">{messages.chat.assistant}</span>
                            <span className="chatMetaTime">{formatElapsed(streamElapsedSeconds)}</span>
                          </div>
                          <div className="chatBubbleContent">
                            {streamingAssistant || (
                              <span className="thinkingText">
                                {streamPhaseLabel(streamPhase, responseProfile)}
                                <span className="thinkingDots" aria-hidden="true"></span>
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
                        placeholder={messages.chat.composerPlaceholder}
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
                        <div className="composerTabs" aria-label={messages.chat.composerOptions}>
                          <button
                            type="button"
                            className="iconCreateBtn composerActionBtn"
                            onClick={() => setIsUploadModalOpen(true)}
                            aria-label={messages.chat.uploadFile}
                            data-tour="upload-btn"
                          >
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="12" y1="5" x2="12" y2="19"></line><line x1="5" y1="12" x2="19" y2="12"></line></svg>
                          </button>
                          <div className="composerSelectGroup" aria-label={messages.chat.responseMode} data-tour="mode-toggle">
                            <ComposerDropdownMenu
                              isOpen={openComposerDropdown === "mode"}
                              label={modeOptions.find((option) => option.value === messageMode)?.label || messages.chat.modeStrict}
                              onToggle={() => setOpenComposerDropdown((current) => current === "mode" ? null : "mode")}
                              options={modeOptions.map((option) => ({
                                value: option.value,
                                label: option.label
                              }))}
                              onSelect={(value) => {
                                setMessageMode(value as "strict" | "unstrict");
                                setOpenComposerDropdown(null);
                              }}
                            />
                            {!canAccessUnstrict && <span className="modeToggleHint">{messages.chat.unstrictDisabled}</span>}
                          </div>
                          <div className="composerSelectGroup" aria-label={messages.chat.responseProfile}>
                            <ComposerDropdownMenu
                              isOpen={openComposerDropdown === "profile"}
                              label={activeResponseProfile.label}
                              onToggle={() => setOpenComposerDropdown((current) => current === "profile" ? null : "profile")}
                              options={responseProfiles.map((profile) => ({
                                value: profile.id,
                                label: profile.label
                              }))}
                              onSelect={(value) => {
                                setResponseProfile(value as ResponseProfile);
                                setOpenComposerDropdown(null);
                              }}
                            />
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
                    <p className="notice">{messages.knowledge.readOnlyNotice}</p>
                  )}

                  {canUploadDocs && (
                    <div className="panelSection">
                      <h2>{messages.knowledge.upload}</h2>
                      <form className="formGrid" onSubmit={onUploadDocument}>
                        <label>
                          {messages.knowledge.titleOptional}
                          <input
                            value={documentTitle}
                            onChange={(event) => setDocumentTitle(event.target.value)}
                            placeholder={messages.knowledge.titlePlaceholder}
                          />
                        </label>

                        <label>
                          {messages.knowledge.file}
                          <input
                            type="file"
                            onChange={(event) => setSelectedFile(event.target.files?.[0] || null)}
                            required
                          />
                        </label>

                        <fieldset>
                          <legend>{messages.knowledge.allowedRoles}</legend>
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
                          {isBusy ? messages.knowledge.uploadBusy : messages.knowledge.uploadSubmit}
                        </button>
                      </form>
                    </div>
                  )}

                  <div className="panelSection">
                    <h2>{messages.knowledge.documents}</h2>
                    {canUploadDocs && (
                      <div className="panelRow">
                        <button type="button" className="btn btnSmall" onClick={() => void onReingestAllDocuments()} disabled={isBusy}>
                          {messages.knowledge.reingestAll}
                        </button>
                      </div>
                    )}
                    <div className="tableWrap">
                      <table>
                        <thead>
                          <tr>
                            <th>{messages.knowledge.name}</th>
                            <th>{messages.knowledge.filename}</th>
                            <th>{messages.knowledge.status}</th>
                            <th>{messages.knowledge.accessRoles}</th>
                            {canUploadDocs && <th>{messages.knowledge.actions}</th>}
                          </tr>
                        </thead>
                        <tbody>
                          {documents.map((entry) => (
                            <tr key={entry.id}>
                              <td>{entry.title}</td>
                              <td>{entry.filename}</td>
                              <td>{getTranslatedStatus(entry.status)}</td>
                              <td>{entry.allowed_role_ids.join(", ")}</td>
                              {canUploadDocs && (
                                <td>
                                  <button
                                    type="button"
                                    className="btn btnSmall"
                                    onClick={() => void onReingestDocument(entry.id)}
                                    disabled={isBusy}
                                  >
                                    {messages.knowledge.reingest}
                                  </button>
                                </td>
                              )}
                            </tr>
                          ))}
                          {documents.length === 0 && (
                            <tr>
                              <td colSpan={canUploadDocs ? 5 : 4}>{messages.knowledge.noDocuments}</td>
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
                  {!canManageUsers && <p className="notice">{messages.users.noAccess}</p>}

                  {canManageUsers && (
                    <div className="panelSection">
                      <h2>{messages.users.title}</h2>
                      <div className="tableWrap">
                        <table>
                          <thead>
                            <tr>
                              <th>{messages.auth.email}</th>
                              <th>{messages.roles.role}</th>
                              <th>{messages.knowledge.status}</th>
                              <th>{messages.users.save}</th>
                            </tr>
                          </thead>
                          <tbody>
                            {users.map((entry) => (
                              <tr key={entry.id}>
                                <td>{entry.email}</td>
                                <td>{entry.role_name}</td>
                                <td>{getTranslatedStatus(entry.status)}</td>
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
                                      {messages.users.save}
                                    </button>
                                  </div>
                                </td>
                              </tr>
                            ))}
                            {users.length === 0 && (
                              <tr>
                                <td colSpan={4}>{messages.users.notFound}</td>
                              </tr>
                            )}
                          </tbody>
                        </table>
                      </div>
                    </div>
                  )}

                  {!canManageRoles && <p className="notice">{messages.roles.noAccess}</p>}
                  {canManageRoles && (
                    <div className="panelSection">
                      <h2>{messages.roles.title}</h2>
                      <div className="formGrid">
                        <label>
                          {messages.roles.roleName}
                          <input
                            type="text"
                            value={roleDraftName}
                            onChange={(event) => setRoleDraftName(event.target.value)}
                            placeholder={messages.roles.rolePlaceholder}
                            disabled={isBusy}
                          />
                        </label>
                        <div>
                          <p className="hint">{messages.roles.permissions}</p>
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
                            {editingRoleID ? messages.roles.update : messages.roles.create}
                          </button>
                          {editingRoleID && (
                            <button type="button" className="btn btnGhost btnSmall" onClick={resetRoleDraft} disabled={isBusy}>
                              {messages.roles.cancel}
                            </button>
                          )}
                        </div>
                      </div>
                      <div className="tableWrap">
                        <table>
                          <thead>
                            <tr>
                              <th>{messages.roles.role}</th>
                              <th>{messages.roles.permissions}</th>
                              <th>{messages.knowledge.actions}</th>
                            </tr>
                          </thead>
                          <tbody>
                            {roles.map((role) => (
                              <tr key={role.id}>
                                <td>
                                  {role.name}
                                  {role.is_default && ` (${messages.roles.defaultSuffix})`}
                                </td>
                                <td>{formatPermissionList(role.permissions)}</td>
                                <td>
                                  <div className="assignRow">
                                    <button type="button" className="btn btnSmall" onClick={() => onStartEditRole(role)} disabled={isBusy || role.is_default}>
                                      {messages.roles.edit}
                                    </button>
                                    <button type="button" className="btn btnGhost btnSmall" onClick={() => void onDeleteRole(role)} disabled={isBusy || role.is_default}>
                                      {messages.roles.delete}
                                    </button>
                                  </div>
                                </td>
                              </tr>
                            ))}
                            {roles.length === 0 && (
                              <tr>
                                <td colSpan={3}>{messages.roles.notFound}</td>
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
                    <h2>{messages.account.title}</h2>
                    <div className="accountGrid">
                      <p>
                        <strong>{messages.account.user}</strong>
                        <br />
                        {user.email}
                      </p>
                      <p>
                        <strong>{messages.account.role}</strong>
                        <br />
                        {user.role_name}
                      </p>
                      <p>
                        <strong>{messages.account.organization}</strong>
                        <br />
                        {user.org_id}
                      </p>
                    </div>
                  </div>

                  <div className="panelSection">
                    <h2>{messages.account.defaultMode}</h2>
                    <p className="hint">{messages.account.defaultModeHint}</p>
                    <div className="settingsRow">
                      <div className="modeToggle" role="tablist" aria-label={messages.account.defaultMode}>
                        <button
                          type="button"
                          className={`modeToggleItem ${(settings?.default_mode || "strict") === "strict" ? "active" : ""}`}
                          onClick={() => void onUpdateDefaultMode("strict")}
                          disabled={isBusy}
                        >
                          {messages.chat.modeStrict}
                        </button>
                        {canAccessUnstrict ? (
                          <button
                            type="button"
                            className={`modeToggleItem ${(settings?.default_mode || "strict") === "unstrict" ? "active" : ""}`}
                            onClick={() => void onUpdateDefaultMode("unstrict")}
                            disabled={isBusy}
                          >
                            {messages.chat.modeUnstrict}
                          </button>
                        ) : (
                          <span className="modeToggleHint">{messages.chat.unstrictDisabled}</span>
                        )}
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
              aria-label={messages.settings.title}
              onClick={(event) => event.stopPropagation()}
            >
              <div className="settingsModalBar">
                <div className="settingsModalTitle">{messages.settings.title}</div>
                <button type="button" className="iconCreateBtn" onClick={() => setIsSettingsOpen(false)} aria-label={messages.settings.close}>
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
                    {messages.settings.knowledge}
                  </button>
                  <button
                    type="button"
                    className={`settingsModalBtn ${settingsTab === "users" ? "active" : ""}`}
                    disabled={!canManageUsers}
                    onClick={() => setSettingsTab("users")}
                  >
                    {messages.settings.users}
                  </button>
                  <button
                    type="button"
                    className={`settingsModalBtn ${settingsTab === "roles" ? "active" : ""}`}
                    disabled={!canManageRoles}
                    onClick={() => setSettingsTab("roles")}
                  >
                    {messages.settings.roles}
                  </button>
                  <button
                    type="button"
                    className={`settingsModalBtn ${settingsTab === "account" ? "active" : ""}`}
                    onClick={() => setSettingsTab("account")}
                  >
                    {messages.settings.account}
                  </button>
                  <button
                    type="button"
                    className="settingsModalBtn settingsModalBtnDanger"
                    onClick={() => void onLogout()}
                    disabled={isBusy}
                  >
                    {messages.settings.logout}
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
                              {messages.knowledge.reingestAll}
                            </button>
                          </div>
                        )}
                        <div className="tableWrap">
                          <table>
                            <thead>
                              <tr>
                                <th>{messages.knowledge.name}</th>
                                <th>{messages.knowledge.filename}</th>
                                <th>{messages.knowledge.status}</th>
                                <th>{messages.knowledge.accessRoles}</th>
                                {canUploadDocs && <th>{messages.knowledge.actions}</th>}
                              </tr>
                            </thead>
                            <tbody>
                              {documents.map((entry) => (
                                <tr key={entry.id}>
                                  <td>{entry.title}</td>
                                  <td>{entry.filename}</td>
                                  <td>{getTranslatedStatus(entry.status)}</td>
                                  <td>{entry.allowed_role_ids.join(", ")}</td>
                                  {canUploadDocs && (
                                    <td>
                                      <button
                                        type="button"
                                        className="btn btnSmall"
                                        onClick={() => void onReingestDocument(entry.id)}
                                        disabled={isBusy}
                                      >
                                        {messages.knowledge.reingest}
                                      </button>
                                    </td>
                                  )}
                                </tr>
                              ))}
                              {documents.length === 0 && (
                                <tr>
                                  <td colSpan={canUploadDocs ? 5 : 4}>{messages.knowledge.noDocuments}</td>
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
                                <th>{messages.auth.email}</th>
                                <th>{messages.roles.role}</th>
                                <th>{messages.knowledge.status}</th>
                                <th>{messages.users.save}</th>
                              </tr>
                            </thead>
                            <tbody>
                              {users.map((entry) => (
                                <tr key={entry.id}>
                                  <td>{entry.email}</td>
                                  <td>{entry.role_name}</td>
                                  <td>{getTranslatedStatus(entry.status)}</td>
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
                                        {messages.users.save}
                                      </button>
                                    </div>
                                  </td>
                                </tr>
                              ))}
                              {users.length === 0 && (
                                <tr>
                                  <td colSpan={4}>{messages.users.notFound}</td>
                                </tr>
                              )}
                            </tbody>
                          </table>
                        </div>
                      ) : (
                        <p className="notice">{messages.users.noAccess}</p>
                      ))}
                    {settingsTab === "roles" &&
                      (canManageRoles ? (
                        <>
                          <div className="formGrid">
                            <label>
                              {messages.roles.roleName}
                              <input
                                type="text"
                                value={roleDraftName}
                                onChange={(event) => setRoleDraftName(event.target.value)}
                                placeholder={messages.roles.rolePlaceholder}
                                disabled={isBusy}
                              />
                            </label>
                            <div>
                              <p className="hint">{messages.roles.permissions}</p>
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
                                {editingRoleID ? messages.roles.update : messages.roles.create}
                              </button>
                              {editingRoleID && (
                                <button type="button" className="btn btnGhost btnSmall" onClick={resetRoleDraft} disabled={isBusy}>
                                  {messages.roles.cancel}
                                </button>
                              )}
                            </div>
                          </div>

                          <div className="tableWrap">
                            <table>
                              <thead>
                                <tr>
                                  <th>{messages.roles.role}</th>
                                  <th>{messages.roles.permissions}</th>
                                  <th>{messages.knowledge.actions}</th>
                                </tr>
                              </thead>
                              <tbody>
                                {roles.map((role) => (
                                  <tr key={role.id}>
                                    <td>
                                      {role.name}
                                      {role.is_default && ` (${messages.roles.defaultSuffix})`}
                                    </td>
                                    <td>{formatPermissionList(role.permissions)}</td>
                                    <td>
                                      <div className="assignRow">
                                        <button
                                          type="button"
                                          className="btn btnSmall"
                                          onClick={() => onStartEditRole(role)}
                                          disabled={isBusy || role.is_default}
                                        >
                                          {messages.roles.edit}
                                        </button>
                                        <button
                                          type="button"
                                          className="btn btnGhost btnSmall"
                                          onClick={() => void onDeleteRole(role)}
                                          disabled={isBusy || role.is_default}
                                        >
                                          {messages.roles.delete}
                                        </button>
                                      </div>
                                    </td>
                                  </tr>
                                ))}
                                {roles.length === 0 && (
                                  <tr>
                                    <td colSpan={3}>{messages.roles.notFound}</td>
                                  </tr>
                                )}
                              </tbody>
                            </table>
                          </div>
                        </>
                      ) : (
                        <p className="notice">{messages.roles.noAccess}</p>
                      ))}
                    {settingsTab === "account" && (
                      <>
                        <div className="panelSection">
                          <h2>{messages.account.title}</h2>
                          <div className="accountGrid">
                            <p>
                              <strong>{messages.account.user}</strong>
                              <br />
                              {user.email}
                            </p>
                            <p>
                              <strong>{messages.account.role}</strong>
                              <br />
                              {user.role_name}
                            </p>
                            <p>
                              <strong>{messages.account.organization}</strong>
                              <br />
                              {user.org_id}
                            </p>
                          </div>
                        </div>
                        <div className="panelSection">
                          <h2>{messages.account.defaultMode}</h2>
                          <p className="hint">{messages.account.defaultModeHint}</p>
                          <div className="settingsRow">
                            <div className="modeToggle" role="tablist" aria-label={messages.account.defaultMode}>
                              <button
                                type="button"
                                className={`modeToggleItem ${(settings?.default_mode || "strict") === "strict" ? "active" : ""}`}
                                onClick={() => void onUpdateDefaultMode("strict")}
                                disabled={isBusy}
                              >
                                {messages.chat.modeStrict}
                              </button>
                              {canAccessUnstrict ? (
                                <button
                                  type="button"
                                  className={`modeToggleItem ${(settings?.default_mode || "strict") === "unstrict" ? "active" : ""}`}
                                  onClick={() => void onUpdateDefaultMode("unstrict")}
                                  disabled={isBusy}
                                >
                                  {messages.chat.modeUnstrict}
                                </button>
                              ) : (
                                <span className="modeToggleHint">{messages.chat.unstrictDisabled}</span>
                              )}
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
              aria-label={messages.uploadModal.title}
              onClick={(event) => event.stopPropagation()}
            >
              <div className="settingsModalBar">
                <div className="settingsModalTitle">{messages.uploadModal.title}</div>
                <button type="button" className="iconCreateBtn" onClick={() => setIsUploadModalOpen(false)} aria-label={messages.uploadModal.close}>
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>
                </button>
              </div>
              <div className="settingsModalContent settingsModalContentUpload">
                {canUploadDocs ? (
                  <form className="formGrid uploadFormCompact" onSubmit={onUploadDocument}>
                    {error && <div className="uploadInlineFeedback uploadInlineFeedbackError">{error}</div>}
                    {message && <div className="uploadInlineFeedback uploadInlineFeedbackSuccess">{message}</div>}
                    <label className="uploadField">
                      {messages.knowledge.titleOptional}
                      <input
                        value={documentTitle}
                        onChange={(event) => setDocumentTitle(event.target.value)}
                        placeholder={messages.knowledge.titlePlaceholder}
                      />
                    </label>
                    <label className="uploadField">
                      {messages.knowledge.file}
                      <input
                        type="file"
                        onChange={(event) => setSelectedFile(event.target.files?.[0] || null)}
                        required
                      />
                    </label>
                    <fieldset className="uploadRolesFieldset">
                      <legend>{messages.knowledge.allowedRoles}</legend>
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
                    <button type="submit" className="btn btnPrimary" disabled={!isUploadReady}>
                      {isBusy ? messages.knowledge.uploadBusy : messages.knowledge.uploadSubmit}
                    </button>
                    {uploadHint && <p className="uploadHint">{uploadHint}</p>}
                  </form>
                ) : (
                  <p className="notice">{messages.uploadModal.noAccess}</p>
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
              aria-label={messages.sourcePreview.open}
              onClick={(event) => event.stopPropagation()}
            >
              <div className="settingsModalBar">
                <div className="settingsModalTitle">{messages.sourcePreview.title}</div>
                <button type="button" className="iconCreateBtn" onClick={closeCitationPreview} aria-label={messages.sourcePreview.close}>
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>
                </button>
              </div>
              <div className="settingsModalContent settingsModalContentUpload">
                <div className="sourcePreviewGrid">
                  <p><strong>{messages.sourcePreview.document}</strong><br />{activeCitation.doc_title}</p>
                  <p><strong>{messages.sourcePreview.file}</strong><br />{activeCitation.doc_filename}</p>
                  <p><strong>Chunk ID</strong><br />{activeCitation.chunk_id}</p>
                  <p><strong>Document ID</strong><br />{activeCitation.document_id}</p>
                  <p><strong>{messages.sourcePreview.page}</strong><br />{activeCitation.page ?? "—"}</p>
                  <p><strong>{messages.sourcePreview.section}</strong><br />{activeCitation.section || "—"}</p>
                </div>
                <div className="sourcePreviewSnippet">
                  <strong>{messages.sourcePreview.snippet}</strong>
                  <p>{activeCitation.snippet || messages.sourcePreview.snippetMissing}</p>
                </div>
                <div className="sourcePreviewGrid">
                  <p><strong>{messages.sourcePreview.scoring}</strong><br />total: {activeCitation.score?.toFixed(4) ?? "—"}</p>
                  <p><strong>Vector</strong><br />{activeCitation.vector_score?.toFixed(4) ?? "—"}</p>
                  <p><strong>Text</strong><br />{activeCitation.text_score?.toFixed(4) ?? "—"}</p>
                  <p><strong>{messages.sourcePreview.documentStatus}</strong><br />{activeCitationDocument ? getTranslatedStatus(activeCitationDocument.status) : messages.sourcePreview.unknown}</p>
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
                    {messages.sourcePreview.goToKnowledge}
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}

        {user && isOnboardingOpen && currentOnboardingStep && (
          <div className="onboardingOverlay" role="dialog" aria-modal="true" aria-label={messages.onboarding.title}>
            {onboardingRect && (
              <div
                className="onboardingHighlight"
                style={{
                  top: onboardingRect.top - 8,
                  left: onboardingRect.left - 8,
                  width: onboardingRect.width + 16,
                  height: onboardingRect.height + 16
                }}
              />
            )}
            <div
              className="onboardingCard"
              style={
                onboardingCardPosition
                  ? {
                      top: onboardingCardPosition.top,
                      left: onboardingCardPosition.left
                    }
                  : {
                      top: "50%",
                      left: "50%",
                      transform: "translate(-50%, -50%)"
                    }
              }
            >
              <div className="onboardingStep">
                {messages.onboarding.step(onboardingStepIndex + 1, onboardingSteps.length)}
              </div>
              <h3>{currentOnboardingStep.title}</h3>
              <p>{currentOnboardingStep.description}</p>
              <div className="onboardingDots" aria-hidden="true">
                {onboardingSteps.map((step, index) => (
                  <span key={step.selector} className={`onboardingDot ${index === onboardingStepIndex ? "active" : ""}`} />
                ))}
              </div>
              <div className="onboardingActions">
                <button type="button" className="btn btnSecondary btnSmall" onClick={() => closeOnboarding(true)}>
                  {messages.onboarding.skip}
                </button>
                <div className="onboardingActionsRight">
                  <button type="button" className="btn btnSecondary btnSmall" onClick={onPrevOnboardingStep} disabled={onboardingStepIndex === 0}>
                    {messages.onboarding.previous}
                  </button>
                  <button type="button" className="btn btnPrimary btnSmall" onClick={onNextOnboardingStep}>
                    {onboardingStepIndex === onboardingSteps.length - 1 ? messages.onboarding.done : messages.onboarding.next}
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

function ComposerDropdownMenu({
  isOpen,
  label,
  options,
  onToggle,
  onSelect
}: {
  isOpen: boolean;
  label: string;
  options: Array<{ value: string; label: string }>;
  onToggle: () => void;
  onSelect: (value: string) => void;
}) {
  return (
    <div className="composerDropdown" data-composer-dropdown>
      <button
        type="button"
        className={`composerDropdownTrigger ${isOpen ? "open" : ""}`}
        onClick={onToggle}
        aria-haspopup="listbox"
        aria-expanded={isOpen}
      >
        <span>{label}</span>
        <svg className="composerDropdownIcon" width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <path d="M6 9L12 15L18 9" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </button>
      {isOpen && (
        <div className="composerDropdownMenu" role="listbox">
          {options.map((option) => (
            <button
              key={option.value}
              type="button"
              className="composerDropdownOption"
              onClick={() => onSelect(option.value)}
            >
              {option.label}
            </button>
          ))}
        </div>
      )}
    </div>
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
    body: options.body ? JSON.stringify(options.body) : undefined,
    signal: options.signal
  });

  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    const reason =
      typeof payload?.error === "string"
        ? payload.error
        : options.requestFailureMessage?.(response.status) || `Request failed: ${response.status}`;
    throw new Error(reason);
  }

  return payload as T;
}

async function apiRequestMultipart<T = unknown>(
  path: string,
  formData: FormData,
  token: string,
  requestFailureMessage?: (status: number) => string
): Promise<T> {
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
    const reason =
      typeof payload?.error === "string"
        ? payload.error
        : requestFailureMessage?.(response.status) || `Request failed: ${response.status}`;
    throw new Error(reason);
  }

  return payload as T;
}

async function streamChatMessage(
  chatID: string,
  body: Record<string, unknown>,
  token: string,
  handlers: {
    signal?: AbortSignal;
    onPhase: (phase: StreamPhase) => void;
    onUserMessage: (message: ChatMessage) => void;
    onAssistantDelta: (delta: string) => void;
    requestFailureMessage?: (status: number) => string;
  }
): Promise<StreamDonePayload> {
  const response = await fetch(`${APIBaseURL}/chats/${chatID}/messages/stream`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`
    },
    credentials: "include",
    body: JSON.stringify(body),
    signal: handlers.signal
  });

  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    const reason =
      typeof payload?.error === "string"
        ? payload.error
        : handlers.requestFailureMessage?.(response.status) || `Request failed: ${response.status}`;
    throw new Error(reason);
  }

  if (!response.body) {
    throw new Error("Streaming response body is missing");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let donePayload: StreamDonePayload | null = null;
  let streamError = "";

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
      if (parsed.event === "phase") {
        const payload = JSON.parse(parsed.data) as { phase?: StreamPhase };
        if (payload.phase) {
          handlers.onPhase(payload.phase);
        }
      }
      if (parsed.event === "done") {
        donePayload = JSON.parse(parsed.data) as StreamDonePayload;
      }
      if (parsed.event === "error") {
        const payload = JSON.parse(parsed.data) as { error?: string };
        streamError = payload.error || "Stream failed";
      }
    }
  }

  if (streamError) {
    throw new Error(streamError);
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

function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}

function errorMessage(value: unknown, fallback: string): string {
  if (value instanceof Error) {
    return value.message;
  }

  return fallback;
}
