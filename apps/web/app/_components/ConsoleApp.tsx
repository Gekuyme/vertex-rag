"use client";

import type { FormEvent, KeyboardEvent as ReactKeyboardEvent, ReactNode } from "react";
import { useEffect, useMemo, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { getMessages } from "../../lib/i18n/messages";
import { useI18n } from "./I18nProvider";
import LocaleSwitcher from "./LocaleSwitcher";
import { useTheme } from "./ThemeProvider";
import type { ThemePreference } from "../../lib/theme";

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
  llm_provider: string;
  llm_model: string;
  updated_at: string;
};

type LLMProviderOption = {
  id: string;
  label: string;
  default_model: string;
  models: string[];
};

type LLMProvidersResponse = {
  default_provider: string;
  providers: LLMProviderOption[];
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
  evidence_span?: string;
  page?: number;
  section?: string;
  parent_id?: string;
  offsets?: Record<string, number>;
  vector_score?: number;
  text_score?: number;
  score?: number;
  dense_rank?: number;
  sparse_rank?: number;
  rrf_score?: number;
  rerank_score?: number;
  query_variant?: string;
  retrievers_used?: string[];
  metadata?: Record<string, unknown>;
};

type GroundingDocument = {
  document_id: string;
  doc_title: string;
  doc_filename: string;
  citation_count: number;
};

type ContradictionSignal = {
  type: string;
  summary: string;
  document_ids: string[];
};

type GroundingSummary = {
  confidence: number;
  confidence_label: string;
  confidence_reasons: string[];
  multi_document: boolean;
  document_count: number;
  documents: GroundingDocument[];
  contradictions: ContradictionSignal[];
};

type MessageResponsePayload = {
  mode: "strict" | "unstrict";
  user_message: ChatMessage;
  assistant_message: ChatMessage;
  citations: Citation[];
  grounding?: GroundingSummary;
};

type ChatMessage = {
  id: string;
  chat_id: string;
  user_id?: string | null;
  role: "user" | "assistant";
  mode: "strict" | "unstrict";
  content: string;
  citations: Citation[];
  response_duration_ms?: number | null;
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
  grounding?: GroundingSummary;
};

type ResponseProfile = "fast" | "balanced" | "thinking";
type StreamPhase = "retrieving" | "drafting" | "finalizing";
type ComposerDropdown = "mode" | "profile" | "model" | null;
type EmptyComposerMenu = "root" | "mode" | "profile" | null;

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

const settingsTabIcons: Record<SettingsTab, ReactNode> = {
  knowledge: <SettingsIconKnowledge />,
  users: <SettingsIconUsers />,
  roles: <SettingsIconRoles />,
  account: <SettingsIconAccount />
};

export default function ConsoleApp({ initialView = "chat" }: ConsoleAppProps) {
  const { locale, messages } = useI18n();
  const { themePreference, setThemePreference, resolvedTheme } = useTheme();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [view, setView] = useState<ConsoleView>(initialView);
  const [isSessionBootstrapping, setIsSessionBootstrapping] = useState(true);
  const [isNavOpen, setIsNavOpen] = useState(false);
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [isUploadModalOpen, setIsUploadModalOpen] = useState(false);
  const [isRenameChatModalOpen, setIsRenameChatModalOpen] = useState(false);
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
  const [openChatActionsFor, setOpenChatActionsFor] = useState<string | null>(null);
  const [chatRenameTarget, setChatRenameTarget] = useState<Chat | null>(null);
  const [chatMessages, setChatMessages] = useState<ChatMessage[]>([]);
  const [assistantGroundingByMessage, setAssistantGroundingByMessage] = useState<Record<string, GroundingSummary>>({});
  const [draftMessage, setDraftMessage] = useState("");
  const [renameChatTitle, setRenameChatTitle] = useState("");
  const [llmProviders, setLLMProviders] = useState<LLMProviderOption[]>([]);
  const [defaultLLMProviderID, setDefaultLLMProviderID] = useState("local");
  const [messageMode, setMessageMode] = useState<"strict" | "unstrict">("strict");
  const [responseProfile, setResponseProfile] = useState<ResponseProfile>("balanced");
  const [openComposerDropdown, setOpenComposerDropdown] = useState<ComposerDropdown>(null);
  const [openEmptyComposerMenu, setOpenEmptyComposerMenu] = useState<EmptyComposerMenu>(null);
  const [streamingAssistant, setStreamingAssistant] = useState("");
  const [streamPhase, setStreamPhase] = useState<StreamPhase | null>(null);
  const [streamStartedAt, setStreamStartedAt] = useState<number | null>(null);
  const [streamElapsedSeconds, setStreamElapsedSeconds] = useState(0);
  const [isStreamingMessage, setIsStreamingMessage] = useState(false);
  const [composerHistoryIndex, setComposerHistoryIndex] = useState<number | null>(null);
  const [composerDraftSnapshot, setComposerDraftSnapshot] = useState("");
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
  const refreshSessionPromiseRef = useRef<Promise<string> | null>(null);
  const streamingAssistantBufferRef = useRef("");
  const streamingAssistantFlushFrameRef = useRef<number | null>(null);
  const shouldStickChatToBottomRef = useRef(true);

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
  const composerHistory = useMemo(
    () =>
      chatMessages
        .filter((entry) => entry.role === "user")
        .map((entry) => entry.content)
        .filter((content) => content.trim() !== ""),
    [chatMessages]
  );
  const assistantResponseDurations = useMemo(() => {
    const durations: Record<string, number> = {};

    for (let index = 0; index < chatMessages.length; index += 1) {
      const entry = chatMessages[index];
      if (entry.role !== "assistant") {
        continue;
      }

      if (typeof entry.response_duration_ms === "number" && entry.response_duration_ms >= 0) {
        durations[entry.id] = Math.max(0, Math.round(entry.response_duration_ms / 1000));
        continue;
      }

      let previousUser: ChatMessage | null = null;
      for (let previousIndex = index - 1; previousIndex >= 0; previousIndex -= 1) {
        const candidate = chatMessages[previousIndex];
        if (candidate.role === "user") {
          previousUser = candidate;
          break;
        }
      }
      if (!previousUser) {
        continue;
      }

      const startedAt = Date.parse(previousUser.created_at);
      const finishedAt = Date.parse(entry.created_at);
      if (Number.isNaN(startedAt) || Number.isNaN(finishedAt) || finishedAt < startedAt) {
        continue;
      }

      durations[entry.id] = Math.max(0, Math.round((finishedAt - startedAt) / 1000));
    }

    return durations;
  }, [chatMessages]);
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
  const themeOptions: Array<{ value: ThemePreference; label: string }> = [
    { value: "system", label: messages.settings.themeSystem },
    { value: "light", label: messages.settings.themeLight },
    { value: "dark", label: messages.settings.themeDark }
  ];
  const modelOptions = useMemo(
    () =>
      llmProviders.flatMap((provider) => {
        if (provider.models.length === 0) {
          return [{ value: encodeLLMSelection(provider.id, ""), label: provider.label }];
        }

        return provider.models.map((model) => ({
          value: encodeLLMSelection(provider.id, model),
          label: provider.models.length === 1 ? provider.label : `${provider.label} · ${model}`
        }));
      }),
    [llmProviders]
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
  const isRenameChatModalPresent = useModalPresence(isRenameChatModalOpen, modalAnimationMs);
  const isCitationPreviewPresent = useModalPresence(isCitationPreviewOpen, modalAnimationMs);
  const currentOnboardingStep = onboardingSteps[onboardingStepIndex] || null;
  const activeResponseProfile = responseProfiles.find((profile) => profile.id === responseProfile) || responseProfiles[1];
  const selectedModelValue = useMemo(() => {
    const providerID = settings?.llm_provider || defaultLLMProviderID;
    const provider = llmProviders.find((entry) => entry.id === providerID) || llmProviders[0] || null;
    if (!provider) {
      return "";
    }
    const model = settings?.llm_model || provider.default_model || provider.models[0] || "";
    return encodeLLMSelection(provider.id, model);
  }, [defaultLLMProviderID, llmProviders, settings?.llm_model, settings?.llm_provider]);
  const currentRequestFailureMessage = messages.feedback.requestFailed;
  const defaultModeValue = settings?.default_mode || "strict";
  const isChatPristine = view === "chat" && chatMessages.length === 0 && !isStreamingMessage;
  const greetingName = useMemo(() => {
    const localPart = user?.email?.split("@")[0]?.trim();
    if (!localPart) {
      return "there";
    }
    const cleaned = localPart
      .replace(/[._-]+/g, " ")
      .trim()
      .split(/\s+/)[0];
    if (!cleaned) {
      return "there";
    }
    return cleaned.charAt(0).toUpperCase() + cleaned.slice(1);
  }, [user?.email]);
  const emptyGreeting = useMemo(() => {
    const greetings = messages.chat.emptyGreetings;
    return greetings[Math.floor(Math.random() * greetings.length)](greetingName);
  }, [greetingName, messages]);
  const settingsTabs: Array<{
    key: SettingsTab;
    label: string;
    disabled?: boolean;
  }> = [
    { key: "knowledge", label: messages.settings.knowledge },
    { key: "users", label: messages.settings.users, disabled: !canManageUsers },
    { key: "roles", label: messages.settings.roles, disabled: !canManageRoles },
    { key: "account", label: messages.settings.account }
  ];
  const settingsTitleByTab: Record<SettingsTab, string> = {
    knowledge: messages.settings.knowledge,
    users: messages.settings.users,
    roles: messages.settings.roles,
    account: messages.settings.account
  };

  const request = async <T,>(path: string, options: RequestOptions = {}) => {
    const nextOptions = {
      ...options,
      requestFailureMessage: options.requestFailureMessage || currentRequestFailureMessage
    };

    try {
      return await apiRequest<T>(path, nextOptions);
    } catch (requestError) {
      if (!shouldRetryWithRefresh(path, nextOptions, requestError)) {
        throw requestError;
      }

      const refreshedAccessToken = await refreshSessionToken();
      return apiRequest<T>(path, {
        ...nextOptions,
        token: refreshedAccessToken
      });
    }
  };

  const requestMultipart = async <T,>(path: string, formData: FormData, accessToken: string) => {
    try {
      return await apiRequestMultipart<T>(path, formData, accessToken, currentRequestFailureMessage);
    } catch (requestError) {
      if (!shouldRetryWithRefresh(path, { method: "POST", token: accessToken }, requestError)) {
        throw requestError;
      }

      const refreshedAccessToken = await refreshSessionToken();
      return apiRequestMultipart<T>(path, formData, refreshedAccessToken, currentRequestFailureMessage);
    }
  };

  const requestStream = async (
    chatID: string,
    body: Record<string, unknown>,
    accessToken: string,
    handlers: {
      signal?: AbortSignal;
      onPhase: (phase: StreamPhase) => void;
      onUserMessage: (message: ChatMessage) => void;
      onAssistantDelta: (delta: string) => void;
    }
  ) => {
    try {
      return await streamChatMessage(chatID, body, accessToken, {
        ...handlers,
        requestFailureMessage: currentRequestFailureMessage
      });
    } catch (requestError) {
      if (!shouldRetryWithRefresh(`/chats/${chatID}/messages/stream`, { method: "POST", token: accessToken }, requestError)) {
        throw requestError;
      }

      const refreshedAccessToken = await refreshSessionToken();
      return streamChatMessage(chatID, body, refreshedAccessToken, {
        ...handlers,
        requestFailureMessage: currentRequestFailureMessage
      });
    }
  };

  useEffect(() => {
    const storedToken = window.localStorage.getItem(accessTokenStorageKey);
    if (storedToken) {
      setToken(storedToken);
    }

    void bootstrapSession(storedToken).finally(() => setIsSessionBootstrapping(false));
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
    if (!message) {
      return;
    }

    const timeoutID = window.setTimeout(() => setMessage(""), 3200);
    return () => window.clearTimeout(timeoutID);
  }, [message]);

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
      if (target.closest("[data-empty-composer-menu]")) {
        return;
      }
      if (target.closest("[data-chat-actions]")) {
        return;
      }
      setOpenComposerDropdown(null);
      setOpenEmptyComposerMenu(null);
      setOpenChatActionsFor(null);
    }

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setOpenComposerDropdown(null);
        setOpenEmptyComposerMenu(null);
        setOpenChatActionsFor(null);
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

  function themeLabel(theme: ThemePreference) {
    return themeOptions.find((entry) => entry.value === theme)?.label || theme;
  }

  function formatPermissionList(permissionKeys: string[]) {
    if (permissionKeys.length === 0) {
      return "—";
    }

    return permissionKeys.map(getTranslatedPermission).join(", ");
  }

  function resizeComposer() {
    const target = composerRef.current;
    if (!target) {
      return;
    }
    target.style.height = "auto";
    target.style.height = `${target.scrollHeight}px`;
  }

  function setComposerDraft(value: string) {
    setDraftMessage(value);
    window.requestAnimationFrame(() => {
      resizeComposer();
    });
  }

  function isCaretAtBoundary(target: HTMLTextAreaElement, direction: "up" | "down") {
    if (target.selectionStart !== target.selectionEnd) {
      return false;
    }

    const caret = target.selectionStart;
    const value = target.value;
    if (direction === "up") {
      return !value.slice(0, caret).includes("\n");
    }
    return !value.slice(caret).includes("\n");
  }

  function navigateComposerHistory(direction: "up" | "down") {
    if (composerHistory.length === 0) {
      return;
    }

    const lastIndex = composerHistory.length - 1;
    if (direction === "up") {
      if (composerHistoryIndex === null) {
        setComposerDraftSnapshot(draftMessage);
        setComposerHistoryIndex(lastIndex);
        setComposerDraft(composerHistory[lastIndex]);
        return;
      }

      const nextIndex = Math.max(0, composerHistoryIndex - 1);
      setComposerHistoryIndex(nextIndex);
      setComposerDraft(composerHistory[nextIndex]);
      return;
    }

    if (composerHistoryIndex === null) {
      return;
    }

    if (composerHistoryIndex >= lastIndex) {
      setComposerHistoryIndex(null);
      setComposerDraft(composerDraftSnapshot);
      return;
    }

    const nextIndex = Math.min(lastIndex, composerHistoryIndex + 1);
    setComposerHistoryIndex(nextIndex);
    setComposerDraft(composerHistory[nextIndex]);
  }

  function onComposerChange(nextValue: string) {
    setDraftMessage(nextValue);
    setComposerHistoryIndex(null);
    setComposerDraftSnapshot(nextValue);
    resizeComposer();
  }

  function onComposerKeyDown(event: ReactKeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      void onSendMessage();
      return;
    }

    if (event.key === "ArrowUp" && isCaretAtBoundary(event.currentTarget, "up")) {
      event.preventDefault();
      navigateComposerHistory("up");
      return;
    }

    if (event.key === "ArrowDown" && isCaretAtBoundary(event.currentTarget, "down")) {
      event.preventDefault();
      navigateComposerHistory("down");
    }
  }

  function isChatNearBottom(target: HTMLDivElement) {
    const thresholdPx = 96;
    return target.scrollHeight - target.scrollTop - target.clientHeight <= thresholdPx;
  }

  function scrollChatToBottom(behavior: ScrollBehavior = "auto") {
    const target = scrollRef.current;
    if (!target) {
      return;
    }
    target.scrollTo({
      top: target.scrollHeight,
      behavior
    });
  }

  useEffect(() => {
    if (view !== "chat") {
      return;
    }

    const target = scrollRef.current;
    if (!target) {
      return;
    }

    const syncScrollState = () => {
      shouldStickChatToBottomRef.current = isChatNearBottom(target);
    };

    syncScrollState();
    target.addEventListener("scroll", syncScrollState);
    return () => target.removeEventListener("scroll", syncScrollState);
  }, [view, activeChatID]);

  useEffect(() => {
    if (view !== "chat") {
      return;
    }

    shouldStickChatToBottomRef.current = true;
    const frameID = window.requestAnimationFrame(() => {
      scrollChatToBottom("auto");
    });
    return () => window.cancelAnimationFrame(frameID);
  }, [view, activeChatID]);

  useEffect(() => {
    if (view !== "chat" || !shouldStickChatToBottomRef.current) {
      return;
    }

    const frameID = window.requestAnimationFrame(() => {
      scrollChatToBottom(isStreamingMessage ? "auto" : "smooth");
    });
    return () => window.cancelAnimationFrame(frameID);
  }, [chatMessages, streamingAssistant, isStreamingMessage, view]);

  useEffect(() => {
    setComposerHistoryIndex(null);
    setComposerDraftSnapshot("");
  }, [activeChatID]);

  useEffect(() => {
    if (!isSettingsOpen && !isUploadModalOpen && !isRenameChatModalOpen && !isCitationPreviewOpen) {
      return;
    }

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setIsSettingsOpen(false);
        setIsUploadModalOpen(false);
        setIsRenameChatModalOpen(false);
        setIsCitationPreviewOpen(false);
      }
    }

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [isSettingsOpen, isUploadModalOpen, isRenameChatModalOpen, isCitationPreviewOpen]);

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

  async function bootstrapSession(accessToken?: string | null) {
    try {
      if (accessToken) {
        const profile = await request<User>("/me", { token: accessToken });
        setUser(profile);

        const [nextSettings] = await Promise.all([
          loadUserPreferences(accessToken),
          loadWorkspace(accessToken, profile),
          bootstrapChats(accessToken)
        ]);
        setSettings(nextSettings);

        setMessage(messages.feedback.signedIn(profile.email));
        setError("");
        return;
      }
    } catch {}

    try {
      const refreshedAccessToken = await refreshSessionToken();
      const refreshedUser = await request<User>("/me", { token: refreshedAccessToken });
      setUser(refreshedUser);

      const [nextSettings] = await Promise.all([
        loadUserPreferences(refreshedAccessToken),
        loadWorkspace(refreshedAccessToken, refreshedUser),
        bootstrapChats(refreshedAccessToken)
      ]);
      setSettings(nextSettings);

      setMessage(messages.feedback.sessionRestored(refreshedUser.email));
      setError("");
    } catch {
      clearSession();
    }
  }

  async function refreshSessionToken(): Promise<string> {
    if (refreshSessionPromiseRef.current) {
      return refreshSessionPromiseRef.current;
    }

    const refreshPromise = apiRequest<AuthResponse>("/auth/refresh", {
      method: "POST",
      requestFailureMessage: currentRequestFailureMessage
    })
      .then((authResponse) => {
        persistAccessToken(authResponse.access_token);
        setUser(authResponse.user);
        setError("");
        return authResponse.access_token;
      })
      .catch((requestError) => {
        clearSession();
        throw requestError;
      })
      .finally(() => {
        refreshSessionPromiseRef.current = null;
      });

    refreshSessionPromiseRef.current = refreshPromise;
    return refreshPromise;
  }

  async function loadUserPreferences(accessToken: string) {
    const [nextSettings, providerCatalog] = await Promise.all([
      request<UserSettings>("/me/settings", { token: accessToken }),
      request<LLMProvidersResponse>("/llm/providers", { token: accessToken })
    ]);

    setLLMProviders(providerCatalog.providers);
    setDefaultLLMProviderID(providerCatalog.default_provider);
    return nextSettings;
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
    setAssistantGroundingByMessage({});
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
        setAssistantGroundingByMessage({});
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
        loadUserPreferences(authResponse.access_token),
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
      setAssistantGroundingByMessage({});
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
    setComposerHistoryIndex(null);
    setComposerDraftSnapshot("");
    resetStreamingAssistant();
    setStreamPhase("retrieving");
    const startedAt = Date.now();
    setStreamStartedAt(startedAt);
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
      payload.locale = locale;
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
      if (response.grounding) {
        setAssistantGroundingByMessage((current) => ({
          ...current,
          [response.assistant_message.id]: response.grounding as GroundingSummary
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
    await onDeleteChatByID(activeChatID);
  }

  async function onDeleteChatByID(chatID: string) {
    if (!token || !chatID) {
      return;
    }

    if (!window.confirm(messages.feedback.deleteChatConfirm)) {
      return;
    }

    setIsBusy(true);
    setMessage("");
    setError("");
    setOpenChatActionsFor(null);
    try {
      await request(`/chats/${chatID}`, {
        method: "DELETE",
        token
      });

      const list = await refreshChats(token);
      if (chatID !== activeChatID) {
        setMessage(messages.feedback.chatDeleted);
        return;
      }

      if (list.length === 0) {
        const created = await request<Chat>("/chats", {
          method: "POST",
          token,
          body: { title: "" }
        });
        setChats([created]);
        setActiveChatID(created.id);
        setChatMessages([]);
        setAssistantGroundingByMessage({});
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

  async function onRenameChat(chat: Chat) {
    setChatRenameTarget(chat);
    setRenameChatTitle(chat.title || messages.shell.newChat);
    setIsRenameChatModalOpen(true);
    setOpenChatActionsFor(null);
  }

  function closeRenameChatModal() {
    setIsRenameChatModalOpen(false);
    setChatRenameTarget(null);
    setRenameChatTitle("");
  }

  async function onSubmitRenameChat(event?: FormEvent<HTMLFormElement>) {
    event?.preventDefault();
    if (!token || !chatRenameTarget) {
      return;
    }

    const currentTitle = chatRenameTarget.title || messages.shell.newChat;
    const nextTitle = renameChatTitle.trim();
    if (!nextTitle || nextTitle === currentTitle) {
      closeRenameChatModal();
      return;
    }

    setIsBusy(true);
    setMessage("");
    setError("");
    try {
      const updated = await request<Chat>(`/chats/${chatRenameTarget.id}`, {
        method: "PATCH",
        token,
        body: { title: nextTitle }
      });
      setChats((current) => current.map((entry) => entry.id === updated.id ? updated : entry));
      setMessage(messages.feedback.chatRenamed);
      closeRenameChatModal();
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

  async function onUpdateLLMSelection(value: string) {
    if (!token) {
      return;
    }

    const nextSelection = decodeLLMSelection(value);
    if (!nextSelection) {
      return;
    }
    if (settings?.llm_provider === nextSelection.provider && settings?.llm_model === nextSelection.model) {
      setOpenComposerDropdown(null);
      return;
    }

    setIsBusy(true);
    setMessage("");
    setError("");
    try {
      const updated = await request<UserSettings>("/me/settings", {
        method: "PATCH",
        token,
        body: {
          llm_provider: nextSelection.provider,
          llm_model: nextSelection.model
        }
      });
      setSettings(updated);
      setOpenComposerDropdown(null);
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
    refreshSessionPromiseRef.current = null;
    window.localStorage.removeItem(accessTokenStorageKey);
    setToken("");
    setUser(null);
    setSettings(null);
    setLLMProviders([]);
    setDefaultLLMProviderID("local");
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
    setAssistantGroundingByMessage({});
    setDraftMessage("");
    resetStreamingAssistant();
    setIsStreamingMessage(false);
    setStreamStartedAt(null);
    setStreamElapsedSeconds(0);
    setIsSessionBootstrapping(false);
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
  const isShowingAuthScreen = !user && !isSessionBootstrapping;

  return (
    <main className={shellClassName}>
      <section className={cardClassName}>
        {message && <div className="message success">{message}</div>}
        {error && <div className="message error">{error}</div>}

        {isSessionBootstrapping && (
          <div className="sessionSplash">
            <div className="sessionSplashCard">
              <p className="sessionSplashEyebrow">Vertex RAG</p>
              <h1 className="sessionSplashTitle">{messages.shell.workspace}</h1>
              <div className="sessionSplashLoader" aria-hidden="true" />
            </div>
          </div>
        )}

        {isShowingAuthScreen && (
          <div className="headerRow">
            <div className="headerRowTop">
              <div>
                <h1>{messages.auth.loginTitle}</h1>
              </div>
            </div>
            <p className="hint">{messages.auth.ownerConsole}</p>
          </div>
        )}

        {isShowingAuthScreen && (
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
                      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                        <path d="M15 4h5v5" />
                        <path d="M14 10 20 4" />
                        <path d="M20 13v5a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h5" />
                      </svg>
                    </button>
                  </div>
                </div>
                <div className="chatList" role="list">
                  {chats.map((chat) => (
                    <div
                      key={chat.id}
                      className={`chatListItem ${chat.id === activeChatID ? "active" : ""}`}
                      role="listitem"
                    >
                      <button
                        type="button"
                        className="chatListSelectBtn"
                        onClick={() => void onSelectChat(chat.id)}
                      >
                        <span className="chatListTitle">{chat.title}</span>
                      </button>
                      <div className="chatActions" data-chat-actions>
                        <button
                          type="button"
                          className="chatActionsTrigger"
                          aria-label={messages.shell.chatActions}
                          onClick={() => setOpenChatActionsFor((current) => current === chat.id ? null : chat.id)}
                        >
                          <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                            <circle cx="5" cy="12" r="1.8" />
                            <circle cx="12" cy="12" r="1.8" />
                            <circle cx="19" cy="12" r="1.8" />
                          </svg>
                        </button>
                        {openChatActionsFor === chat.id && (
                          <div className="chatActionsMenu" role="menu">
                            <button
                              type="button"
                              className="chatActionsOption"
                              onClick={() => void onRenameChat(chat)}
                            >
                              {messages.shell.renameChat}
                            </button>
                            <button
                              type="button"
                              className="chatActionsOption chatActionsOptionDanger"
                              onClick={() => void onDeleteChatByID(chat.id)}
                            >
                              {messages.shell.deleteChat}
                            </button>
                          </div>
                        )}
                      </div>
                    </div>
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
                  {view === "chat" && (
                    <ComposerDropdownMenu
                      isOpen={openComposerDropdown === "model"}
                      label={modelOptions.find((option) => option.value === selectedModelValue)?.label || modelOptions[0]?.label || messages.chat.model}
                      onToggle={() => setOpenComposerDropdown((current) => current === "model" ? null : "model")}
                      options={modelOptions}
                      onSelect={onUpdateLLMSelection}
                      menuPlacement="bottom"
                      ariaLabel={messages.chat.model}
                      triggerClassName="headerModelTrigger"
                      menuClassName="headerModelMenu"
                      optionClassName="headerModelOption"
                    />
                  )}
                  {view === "knowledge" && <span>{messages.shell.knowledge}</span>}
                  {view === "users" && <span>{messages.shell.users}</span>}
                  {view === "account" && <span>{messages.shell.account}</span>}
                </div>
                <div className="mainHeaderRight"></div>
              </header>

              {view === "chat" && (
                <div className="chatPane">
                  <div className="chatScroll" ref={scrollRef}>
                    {chatMessages.map((entry) => (
                      <div
                        key={entry.id}
                        className={`chatMessageRow ${entry.role === "user" ? "fromUser" : "fromAssistant"} ${entry.client_status === "failed" ? "isFailed" : ""}`}
                      >
                        {entry.role === "assistant" && assistantResponseDurations[entry.id] !== undefined && (
                          <ChatWorkedDivider label={messages.chat.workedFor(formatElapsed(assistantResponseDurations[entry.id]))} />
                        )}
                        <div className="chatBubble">
                          <div className="chatBubbleContent">
                            <ChatMessageContent role={entry.role} content={entry.content} />
                          </div>
                          {entry.role === "user" && entry.client_status === "failed" && (
                            <div className="chatBubbleHint">{messages.chat.failedDelivery}</div>
                          )}
                          {entry.role === "assistant" &&
                            entry.mode === "strict" &&
                            noKnowledgeMessages.includes(entry.content.trim()) &&
                            (entry.citations?.length || 0) === 0 && (
                              <div className="chatBubbleHint">{messages.chat.noKnowledgeHint}</div>
                            )}
                          {entry.role === "assistant" && assistantGroundingByMessage[entry.id] && (
                            <ChatGroundingPanel grounding={assistantGroundingByMessage[entry.id]} />
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
                                      <div className="sourceSnippet">
                                        {renderCitationSnippet(citation)}
                                      </div>
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
                            <span className="chatMetaTime">{formatElapsed(streamElapsedSeconds)}</span>
                          </div>
                          <div className="chatBubbleContent">
                            {streamingAssistant ? (
                              <ChatMessageContent role="assistant" content={streamingAssistant} />
                            ) : (
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

                  {!isChatPristine && <div className="composerCloud" aria-hidden="true" />}
                  <form className={`composer ${isChatPristine ? "composerEmpty" : ""}`} onSubmit={(event) => void onSendMessage(event)}>
                    {isChatPristine && <div className="chatGreeting">{emptyGreeting}</div>}
                    <div className="composerCard">
                      {isChatPristine ? (
                        <div className="composerEmptyRow">
                          <div className="emptyComposerMenuWrap" data-empty-composer-menu>
                            <button
                              type="button"
                              className="iconCreateBtn composerActionBtn composerEmptyTrigger"
                              onClick={() => setOpenEmptyComposerMenu((current) => current === "root" ? null : "root")}
                              aria-label={messages.chat.composerOptions}
                              data-tour="upload-btn"
                            >
                              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                                <line x1="12" y1="5" x2="12" y2="19"></line>
                                <line x1="5" y1="12" x2="19" y2="12"></line>
                              </svg>
                            </button>
                            {openEmptyComposerMenu && (
                              <div className="emptyComposerMenu" role="menu">
                                <button
                                  type="button"
                                  className="emptyComposerMenuOption"
                                  onClick={() => {
                                    setIsUploadModalOpen(true);
                                    setOpenEmptyComposerMenu(null);
                                  }}
                                >
                                  <span className="emptyComposerMenuOptionIcon" aria-hidden="true">
                                    <EmptyMenuFilesIcon />
                                  </span>
                                  <span>{messages.chat.uploadFile}</span>
                                </button>
                                <button
                                  type="button"
                                  className={`emptyComposerMenuOption ${openEmptyComposerMenu === "mode" ? "active" : ""}`}
                                  onClick={() => setOpenEmptyComposerMenu((current) => current === "mode" ? "root" : "mode")}
                                >
                                  <span className="emptyComposerMenuOptionIcon" aria-hidden="true">
                                    <EmptyMenuModeIcon />
                                  </span>
                                  <span>{modeOptions.find((option) => option.value === messageMode)?.label || messages.chat.modeStrict}</span>
                                  <span className="emptyComposerMenuOptionChevron" aria-hidden="true">
                                    <EmptyMenuChevronIcon />
                                  </span>
                                </button>
                                <button
                                  type="button"
                                  className={`emptyComposerMenuOption ${openEmptyComposerMenu === "profile" ? "active" : ""}`}
                                  onClick={() => setOpenEmptyComposerMenu((current) => current === "profile" ? "root" : "profile")}
                                >
                                  <span className="emptyComposerMenuOptionIcon" aria-hidden="true">
                                    <EmptyMenuProfileIcon />
                                  </span>
                                  <span>{activeResponseProfile.label}</span>
                                  <span className="emptyComposerMenuOptionChevron" aria-hidden="true">
                                    <EmptyMenuChevronIcon />
                                  </span>
                                </button>
                                {openEmptyComposerMenu === "mode" && (
                                  <div className="emptyComposerSubmenu" role="menu">
                                    {modeOptions.map((option) => (
                                      <button
                                        key={option.value}
                                        type="button"
                                        className="emptyComposerMenuOption"
                                        onClick={() => {
                                          setMessageMode(option.value);
                                          setOpenEmptyComposerMenu("root");
                                        }}
                                      >
                                        {option.label}
                                      </button>
                                    ))}
                                  </div>
                                )}
                                {openEmptyComposerMenu === "profile" && (
                                  <div className="emptyComposerSubmenu" role="menu">
                                    {responseProfiles.map((profile) => (
                                      <button
                                        key={profile.id}
                                        type="button"
                                        className="emptyComposerMenuOption"
                                        onClick={() => {
                                          setResponseProfile(profile.id);
                                          setOpenEmptyComposerMenu("root");
                                        }}
                                      >
                                        {profile.label}
                                      </button>
                                    ))}
                                  </div>
                                )}
                              </div>
                            )}
                          </div>
                          <textarea
                            ref={composerRef}
                            value={draftMessage}
                            onChange={(event) => {
                              onComposerChange(event.target.value);
                            }}
                            placeholder={messages.chat.composerPlaceholder}
                            rows={1}
                            className="composerInput composerInputEmpty"
                            onKeyDown={onComposerKeyDown}
                          />
                          <button
                            type="submit"
                            className="btn btnPrimary composerSendBtn"
                            disabled={isBusy || isStreamingMessage || draftMessage.trim() === ""}
                          >
                            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.1" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                              <path d="M12 19V5" />
                              <path d="m6 11 6-6 6 6" />
                            </svg>
                          </button>
                        </div>
                      ) : (
                        <>
                          <textarea
                            ref={composerRef}
                            value={draftMessage}
                            onChange={(event) => {
                              onComposerChange(event.target.value);
                            }}
                            placeholder={messages.chat.composerPlaceholder}
                            rows={1}
                            className="composerInput"
                            onKeyDown={onComposerKeyDown}
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
                            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.1" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                              <path d="M12 19V5" />
                              <path d="m6 11 6-6 6 6" />
                            </svg>
                          </button>
                          </div>
                        </>
                      )}
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
              <div className="settingsModalShell">
                <aside className="settingsModalSidebar">
                  <button type="button" className="settingsModalCloseBtn" onClick={() => setIsSettingsOpen(false)} aria-label={messages.settings.close}>
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>
                  </button>
                  <div className="settingsModalNav settingsModalNavVertical">
                    {settingsTabs.map((tab) => (
                      <button
                        key={tab.key}
                        type="button"
                        className={`settingsModalBtn ${settingsTab === tab.key ? "active" : ""}`}
                        disabled={tab.disabled}
                        onClick={() => setSettingsTab(tab.key)}
                      >
                        <span className="settingsModalBtnIcon" aria-hidden="true">
                          {settingsTabIcons[tab.key]}
                        </span>
                        <span>{tab.label}</span>
                      </button>
                    ))}
                    <button
                      type="button"
                      className="settingsModalBtn settingsModalBtnDanger"
                      onClick={() => void onLogout()}
                      disabled={isBusy}
                    >
                      <span className="settingsModalBtnIcon" aria-hidden="true">
                        <SettingsIconLogout />
                      </span>
                      <span>{messages.settings.logout}</span>
                    </button>
                  </div>
                </aside>
                <div className="settingsModalPane">
                  <div className="settingsPaneContent" key={settingsTab}>
                    <div className="settingsModalHeader">
                      <div className="settingsModalTitle">{settingsTitleByTab[settingsTab]}</div>
                    </div>
                    {settingsTab === "knowledge" && (
                      <div className="settingsSections">
                        <SettingsSection
                          title={messages.settings.documentLibrary}
                          description={messages.settings.knowledgeHint}
                        >
                          {canUploadDocs && (
                            <SettingsRow
                              label={messages.knowledge.reingestAll}
                              description={messages.settings.knowledgeHint}
                              action={
                                <button
                                  type="button"
                                  className="btn btnSmall settingsActionBtn"
                                  onClick={() => void onReingestAllDocuments()}
                                  disabled={isBusy}
                                >
                                  {messages.knowledge.reingestAll}
                                </button>
                              }
                            />
                          )}
                          {documents.length === 0 ? (
                            <SettingsEmptyState>{messages.knowledge.noDocuments}</SettingsEmptyState>
                          ) : (
                            documents.map((entry) => (
                              <SettingsRow
                                key={entry.id}
                                label={entry.title || entry.filename}
                                description={`${messages.knowledge.file}: ${entry.filename}`}
                                meta={[
                                  getTranslatedStatus(entry.status),
                                  entry.allowed_role_ids.length > 0
                                    ? `${messages.settings.uploadAllowedRoles}: ${entry.allowed_role_ids
                                        .map((roleID) => roles.find((role) => role.id === roleID)?.name || String(roleID))
                                        .join(", ")}`
                                    : messages.settings.noRolesSelected
                                ]}
                                action={
                                  canUploadDocs ? (
                                    <button
                                      type="button"
                                      className="btn btnSecondary btnSmall settingsActionBtn"
                                      onClick={() => void onReingestDocument(entry.id)}
                                      disabled={isBusy}
                                    >
                                      {messages.knowledge.reingest}
                                    </button>
                                  ) : undefined
                                }
                              />
                            ))
                          )}
                        </SettingsSection>
                      </div>
                    )}
                    {settingsTab === "users" &&
                      (canManageUsers ? (
                        <div className="settingsSections">
                          <SettingsSection
                            title={messages.users.title}
                            description={messages.settings.manageUsersHint}
                          >
                            {users.length === 0 ? (
                              <SettingsEmptyState>{messages.users.notFound}</SettingsEmptyState>
                            ) : (
                              users.map((entry) => (
                                <SettingsRow
                                  key={entry.id}
                                  label={entry.email}
                                  description={`${messages.roles.role}: ${entry.role_name}`}
                                  meta={[getTranslatedStatus(entry.status)]}
                                  action={
                                    <div className="settingsInlineControls">
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
                                        className="btn btnSmall settingsActionBtn"
                                        onClick={() => void onChangeRole(entry.id)}
                                        disabled={isBusy}
                                      >
                                        {messages.users.save}
                                      </button>
                                    </div>
                                  }
                                />
                              ))
                            )}
                          </SettingsSection>
                        </div>
                      ) : (
                        <p className="notice">{messages.users.noAccess}</p>
                      ))}
                    {settingsTab === "roles" &&
                      (canManageRoles ? (
                        <div className="settingsSections">
                          <SettingsSection
                            title={editingRoleID ? messages.roles.update : messages.settings.createRole}
                            description={messages.settings.manageRolesHint}
                          >
                            <div className="settingsFormCard">
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
                                <div className="rolesGrid settingsPermissionGrid">
                                  {rolePermissionOptions.map((permission) => (
                                    <label key={permission.key} className="roleCheck settingsPermissionItem">
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
                              <div className="settingsInlineControls">
                                <button type="button" className="btn btnPrimary btnSmall settingsActionBtn" onClick={() => void onSaveRole()} disabled={isBusy}>
                                  {editingRoleID ? messages.roles.update : messages.roles.create}
                                </button>
                                {editingRoleID && (
                                  <button type="button" className="btn btnGhost btnSmall settingsActionBtn" onClick={resetRoleDraft} disabled={isBusy}>
                                    {messages.roles.cancel}
                                  </button>
                                )}
                              </div>
                            </div>
                          </SettingsSection>

                          <SettingsSection
                            title={messages.settings.roleDirectory}
                            description={messages.settings.manageRolesHint}
                          >
                            {roles.length === 0 ? (
                              <SettingsEmptyState>{messages.roles.notFound}</SettingsEmptyState>
                            ) : (
                              roles.map((role) => (
                                <SettingsRow
                                  key={role.id}
                                  label={`${role.name}${role.is_default ? ` (${messages.roles.defaultSuffix})` : ""}`}
                                  description={role.permissions.length > 0 ? formatPermissionList(role.permissions) : messages.settings.noPermissions}
                                  action={
                                    <div className="settingsInlineControls">
                                      <button
                                        type="button"
                                        className="btn btnSmall settingsActionBtn"
                                        onClick={() => onStartEditRole(role)}
                                        disabled={isBusy || role.is_default}
                                      >
                                        {messages.roles.edit}
                                      </button>
                                      <button
                                        type="button"
                                        className="btn btnGhost btnSmall settingsActionBtn"
                                        onClick={() => void onDeleteRole(role)}
                                        disabled={isBusy || role.is_default}
                                      >
                                        {messages.roles.delete}
                                      </button>
                                    </div>
                                  }
                                />
                              ))
                            )}
                          </SettingsSection>
                        </div>
                      ) : (
                        <p className="notice">{messages.roles.noAccess}</p>
                      ))}
                    {settingsTab === "account" && (
                      <div className="settingsSections">
                        <SettingsSection
                          title={messages.settings.appearance}
                          description={messages.settings.accountHint}
                        >
                          <SettingsRow
                            label={messages.settings.theme}
                            description={messages.settings.themeHint}
                            action={
                              <div className="modeToggle" role="tablist" aria-label={messages.settings.theme}>
                                {themeOptions.map((option) => (
                                  <button
                                    key={option.value}
                                    type="button"
                                    className={`modeToggleItem ${themePreference === option.value ? "active" : ""}`}
                                    onClick={() => setThemePreference(option.value)}
                                  >
                                    {option.label}
                                  </button>
                                ))}
                              </div>
                            }
                          />
                          <SettingsRow
                            label={messages.localeSwitcherLabel}
                            action={<LocaleSwitcher />}
                          />
                        </SettingsSection>

                        <SettingsSection
                          title={messages.account.title}
                          description={messages.account.defaultModeHint}
                        >
                          <SettingsRow
                            label={messages.account.user}
                            value={user.email}
                          />
                          <SettingsRow
                            label={messages.account.role}
                            value={user.role_name}
                          />
                          <SettingsRow
                            label={messages.account.organization}
                            value={user.org_id}
                          />
                          <SettingsRow
                            label={messages.account.defaultMode}
                            description={messages.account.defaultModeHint}
                            action={
                              <div className="modeToggle" role="tablist" aria-label={messages.account.defaultMode}>
                                <button
                                  type="button"
                                  className={`modeToggleItem ${defaultModeValue === "strict" ? "active" : ""}`}
                                  onClick={() => void onUpdateDefaultMode("strict")}
                                  disabled={isBusy}
                                >
                                  {messages.chat.modeStrict}
                                </button>
                                {canAccessUnstrict ? (
                                  <button
                                    type="button"
                                    className={`modeToggleItem ${defaultModeValue === "unstrict" ? "active" : ""}`}
                                    onClick={() => void onUpdateDefaultMode("unstrict")}
                                    disabled={isBusy}
                                  >
                                    {messages.chat.modeUnstrict}
                                  </button>
                                ) : (
                                  <span className="modeToggleHint">{messages.chat.unstrictDisabled}</span>
                                )}
                              </div>
                            }
                          />
                        </SettingsSection>
                      </div>
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
              <div className="settingsModalUploadShell">
                <div className="settingsModalUploadHeader">
                  <button type="button" className="settingsModalCloseBtn" onClick={() => setIsUploadModalOpen(false)} aria-label={messages.uploadModal.close}>
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>
                  </button>
                  <div className="settingsModalTitle">{messages.uploadModal.title}</div>
                </div>
                <div className="settingsModalContent settingsModalContentUpload">
                  {canUploadDocs ? (
                    <form className="formGrid uploadFormCompact" onSubmit={onUploadDocument}>
                      {error && <div className="uploadInlineFeedback uploadInlineFeedbackError">{error}</div>}
                      {message && <div className="uploadInlineFeedback uploadInlineFeedbackSuccess">{message}</div>}

                      <div className="settingsSections">
                        <SettingsSection title={messages.uploadModal.title}>
                          <SettingsRow
                            label={messages.knowledge.titleOptional}
                            action={
                              <label className="uploadField uploadFieldInline">
                                <input
                                  value={documentTitle}
                                  onChange={(event) => setDocumentTitle(event.target.value)}
                                  placeholder={messages.knowledge.titlePlaceholder}
                                />
                              </label>
                            }
                          />
                          <SettingsRow
                            label={messages.knowledge.file}
                            action={
                              <label className="uploadField uploadFieldInline">
                                <input
                                  type="file"
                                  onChange={(event) => setSelectedFile(event.target.files?.[0] || null)}
                                  required
                                />
                              </label>
                            }
                          />
                        </SettingsSection>

                        <SettingsSection title={messages.knowledge.allowedRoles}>
                          <fieldset className="uploadRolesFieldset">
                            <div className="rolesGrid uploadRolesGrid">
                              {roles.map((role) => (
                                <label key={role.id} className="roleCheck settingsPermissionItem">
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
                        </SettingsSection>
                      </div>

                      <div className="uploadFooter">
                        {uploadHint && <p className="uploadHint">{uploadHint}</p>}
                        <button type="submit" className="btn btnPrimary uploadSubmitBtn" disabled={!isUploadReady}>
                          {isBusy ? messages.knowledge.uploadBusy : messages.knowledge.uploadSubmit}
                        </button>
                      </div>
                    </form>
                  ) : (
                    <p className="notice">{messages.uploadModal.noAccess}</p>
                  )}
                </div>
              </div>
            </div>
          </div>
        )}

        {user && isRenameChatModalPresent && (
          <div
            className={`settingsModalOverlay ${isRenameChatModalOpen ? "modalOverlayVisible" : "modalOverlayHidden"}`}
            onClick={closeRenameChatModal}
          >
            <div
              className={`settingsModal settingsModalRename ${isRenameChatModalOpen ? "modalCardVisible" : "modalCardHidden"}`}
              role="dialog"
              aria-modal="true"
              aria-label={messages.shell.renameChat}
              onClick={(event) => event.stopPropagation()}
            >
              <div className="settingsModalUploadShell">
                <div className="settingsModalUploadHeader">
                  <button type="button" className="settingsModalCloseBtn" onClick={closeRenameChatModal} aria-label={messages.settings.close}>
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>
                  </button>
                  <div className="settingsModalTitle">{messages.shell.renameChat}</div>
                </div>
                <div className="settingsModalContent settingsModalContentUpload">
                  <form className="formGrid uploadFormCompact" onSubmit={onSubmitRenameChat}>
                    <SettingsSection title={messages.shell.renameChat}>
                      <SettingsRow
                        label={messages.shell.renameChat}
                        action={
                          <label className="uploadField uploadFieldInline">
                            <input
                              value={renameChatTitle}
                              onChange={(event) => setRenameChatTitle(event.target.value)}
                              placeholder={messages.shell.newChat}
                              autoFocus
                            />
                          </label>
                        }
                      />
                    </SettingsSection>
                    <div className="uploadFooter">
                      <button
                        type="button"
                        className="btn btnSecondary uploadSubmitBtn"
                        onClick={closeRenameChatModal}
                        disabled={isBusy}
                      >
                        {messages.roles.cancel}
                      </button>
                      <button type="submit" className="btn btnPrimary uploadSubmitBtn" disabled={isBusy || renameChatTitle.trim() === ""}>
                        {isBusy ? messages.auth.submitBusy : messages.users.save}
                      </button>
                    </div>
                  </form>
                </div>
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
                  <p><strong>{messages.sourcePreview.parentSection}</strong><br />{activeCitation.parent_id || "—"}</p>
                  <p><strong>{messages.sourcePreview.offsets}</strong><br />{activeCitation.offsets ? `${activeCitation.offsets.start ?? "—"}-${activeCitation.offsets.end ?? "—"}` : "—"}</p>
                </div>
                <div className="sourcePreviewSnippet">
                  <strong>{messages.sourcePreview.snippet}</strong>
                  <p>{renderCitationSnippet(activeCitation, true)}</p>
                </div>
                <div className="sourcePreviewGrid">
                  <p><strong>{messages.sourcePreview.scoring}</strong><br />total: {activeCitation.score?.toFixed(4) ?? "—"}</p>
                  <p><strong>Vector</strong><br />{activeCitation.vector_score?.toFixed(4) ?? "—"}</p>
                  <p><strong>Text</strong><br />{activeCitation.text_score?.toFixed(4) ?? "—"}</p>
                  <p><strong>{messages.sourcePreview.denseRank}</strong><br />{activeCitation.dense_rank ?? "—"}</p>
                  <p><strong>{messages.sourcePreview.sparseRank}</strong><br />{activeCitation.sparse_rank ?? "—"}</p>
                  <p><strong>{messages.sourcePreview.rrfScore}</strong><br />{activeCitation.rrf_score?.toFixed(4) ?? "—"}</p>
                  <p><strong>{messages.sourcePreview.rerankScore}</strong><br />{activeCitation.rerank_score?.toFixed(4) ?? "—"}</p>
                  <p><strong>{messages.sourcePreview.queryVariant}</strong><br />{activeCitation.query_variant || "—"}</p>
                  <p><strong>{messages.sourcePreview.retrievers}</strong><br />{activeCitation.retrievers_used?.join(", ") || "—"}</p>
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
  onSelect,
  triggerClassName,
  menuClassName,
  optionClassName,
  menuPlacement = "top",
  ariaLabel
}: {
  isOpen: boolean;
  label: string;
  options: Array<{ value: string; label: string }>;
  onToggle: () => void;
  onSelect: (value: string) => void;
  triggerClassName?: string;
  menuClassName?: string;
  optionClassName?: string;
  menuPlacement?: "top" | "bottom";
  ariaLabel?: string;
}) {
  return (
    <div className="composerDropdown" data-composer-dropdown>
      <button
        type="button"
        className={`composerDropdownTrigger ${triggerClassName || ""} ${isOpen ? "open" : ""}`.trim()}
        onClick={onToggle}
        aria-haspopup="listbox"
        aria-expanded={isOpen}
        aria-label={ariaLabel || label}
      >
        <span>{label}</span>
        <svg className="composerDropdownIcon" width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <path d="M6 9L12 15L18 9" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </button>
      {isOpen && (
        <div
          className={`composerDropdownMenu ${menuPlacement === "bottom" ? "composerDropdownMenuBottom" : ""} ${menuClassName || ""}`.trim()}
          role="listbox"
        >
          {options.map((option) => (
            <button
              key={option.value}
              type="button"
              className={`composerDropdownOption ${optionClassName || ""}`.trim()}
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

function ChatMessageContent({
  role,
  content
}: {
  role: "user" | "assistant";
  content: string;
}) {
  if (role === "user") {
    return <span className="chatPlainText">{content}</span>;
  }

  return (
    <div className="chatRichText">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          a(props) {
            return <a {...props} target="_blank" rel="noreferrer" />;
          },
          pre(props) {
            const { className, ...rest } = props;
            return <pre className={["chatCodeBlock", className].filter(Boolean).join(" ")} {...rest} />;
          },
          code(props) {
            const { className, children, ...rest } = props;
            const value = String(children).replace(/\n$/, "");
            const isBlock =
              (typeof className === "string" && className.includes("language-")) || value.includes("\n");
            if (isBlock) {
              return (
                <code className={className} {...rest}>
                  {value}
                </code>
              );
            }
            return (
              <code className="chatInlineCode" {...rest}>
                {value}
              </code>
            );
          }
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}

function ChatWorkedDivider({ label }: { label: string }) {
  return (
    <div className="chatWorkedDivider" aria-label={label}>
      <span className="chatWorkedDividerLine" aria-hidden="true"></span>
      <span className="chatWorkedDividerLabel">{label}</span>
      <span className="chatWorkedDividerLine" aria-hidden="true"></span>
    </div>
  );
}

function ChatGroundingPanel({ grounding }: { grounding: GroundingSummary }) {
  return (
    <div className="chatGroundingPanel">
      <div className="chatGroundingHeader">
        <span className={`chatGroundingBadge is-${grounding.confidence_label}`}>{grounding.confidence_label}</span>
        <span className="chatGroundingScore">{Math.round((grounding.confidence || 0) * 100)}%</span>
        <span className="chatGroundingMeta">
          {grounding.document_count} doc{grounding.document_count === 1 ? "" : "s"}
        </span>
      </div>
      {grounding.confidence_reasons.length > 0 && (
        <div className="chatGroundingReasons">
          {grounding.confidence_reasons.map((reason) => (
            <span key={reason} className="chatGroundingPill">{reason}</span>
          ))}
        </div>
      )}
      {grounding.documents.length > 0 && (
        <div className="chatGroundingDocs">
          {grounding.documents.map((document) => (
            <span key={document.document_id} className="chatGroundingDoc">
              {document.doc_title || document.doc_filename} · {document.citation_count}
            </span>
          ))}
        </div>
      )}
      {grounding.contradictions.length > 0 && (
        <div className="chatGroundingAlert">
          {grounding.contradictions[0].summary}
        </div>
      )}
    </div>
  );
}

function renderCitationSnippet(citation: Citation, expanded = false): ReactNode {
  const snippet = (citation.snippet || "").trim();
  const evidence = (citation.evidence_span || "").trim();

  if (!snippet) {
    return null;
  }
  if (!evidence) {
    return snippet;
  }

  const normalizedSnippet = snippet.toLowerCase();
  const normalizedEvidence = evidence.toLowerCase();
  const matchIndex = normalizedSnippet.indexOf(normalizedEvidence);

  if (matchIndex === -1) {
    return (
      <>
        <span>{snippet}</span>
        <span className="sourceEvidenceCaption">{expanded ? "Evidence span" : "Evidence"}:</span>{" "}
        <mark className="sourceEvidenceMark">{evidence}</mark>
      </>
    );
  }

  const before = snippet.slice(0, matchIndex);
  const matched = snippet.slice(matchIndex, matchIndex + evidence.length);
  const after = snippet.slice(matchIndex + evidence.length);

  return (
    <>
      {before}
      <mark className="sourceEvidenceMark">{matched}</mark>
      {after}
    </>
  );
}

function SettingsSection({
  title,
  description,
  children
}: {
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <section className="settingsGroup">
      <div className="settingsGroupHeader">
        <h2>{title}</h2>
        {description ? <p className="hint">{description}</p> : null}
      </div>
      <div className="settingsGroupBody">{children}</div>
    </section>
  );
}

function SettingsRow({
  label,
  description,
  value,
  meta,
  action
}: {
  label: string;
  description?: string;
  value?: ReactNode;
  meta?: string[];
  action?: ReactNode;
}) {
  return (
    <div className="settingsListRow">
      <div className="settingsListContent">
        <div className="settingsListLabel">{label}</div>
        {description ? <p className="settingsListDescription">{description}</p> : null}
        {meta && meta.length > 0 ? (
          <div className="settingsMetaList">
            {meta.filter(Boolean).map((entry) => (
              <span key={entry} className="settingsMetaPill">
                {entry}
              </span>
            ))}
          </div>
        ) : null}
      </div>
      <div className="settingsListAction">
        {action || (value ? <div className="settingsValueText">{value}</div> : null)}
      </div>
    </div>
  );
}

function SettingsEmptyState({ children }: { children: ReactNode }) {
  return <div className="settingsEmptyState">{children}</div>;
}

function SettingsIconKnowledge() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round">
      <path d="M4 19.5V6.8a2 2 0 0 1 2-2h11.2a2 2 0 0 1 2 2v12.7" />
      <path d="M8 8h8" />
      <path d="M8 12h8" />
      <path d="M8 16h5" />
    </svg>
  );
}

function SettingsIconUsers() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round">
      <path d="M16 21v-2a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4v2" />
      <circle cx="9.5" cy="7" r="3.5" />
      <path d="M20 21v-2a4 4 0 0 0-3-3.87" />
      <path d="M15 3.13a3.5 3.5 0 0 1 0 6.74" />
    </svg>
  );
}

function SettingsIconRoles() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 3 4 7.5v9L12 21l8-4.5v-9L12 3Z" />
      <path d="M12 12 4 7.5" />
      <path d="m12 12 8-4.5" />
      <path d="M12 12v9" />
    </svg>
  );
}

function SettingsIconAccount() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="8" r="4" />
      <path d="M4 20a8 8 0 0 1 16 0" />
    </svg>
  );
}

function SettingsIconLogout() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round">
      <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
      <path d="m16 17 5-5-5-5" />
      <path d="M21 12H9" />
    </svg>
  );
}

function EmptyMenuFilesIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 5v14" />
      <path d="M5 12h14" />
    </svg>
  );
}

function EmptyMenuModeIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round">
      <path d="M7 7h10" />
      <path d="M7 17h10" />
      <path d="M9.5 7A2.5 2.5 0 1 1 7 9.5" />
      <path d="M14.5 17A2.5 2.5 0 1 0 17 14.5" />
    </svg>
  );
}

function EmptyMenuProfileIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round">
      <path d="M13 3 4 14h6l-1 7 9-11h-6l1-7Z" />
    </svg>
  );
}

function EmptyMenuChevronIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="m9 6 6 6-6 6" />
    </svg>
  );
}

class APIRequestError extends Error {
  status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = "APIRequestError";
    this.status = status;
  }
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
    throw new APIRequestError(reason, response.status);
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
    throw new APIRequestError(reason, response.status);
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
    throw new APIRequestError(reason, response.status);
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

function encodeLLMSelection(provider: string, model: string): string {
  return `${provider}::${model}`;
}

function decodeLLMSelection(value: string): { provider: string; model: string } | null {
  const separatorIndex = value.indexOf("::");
  if (separatorIndex === -1) {
    return null;
  }
  return {
    provider: value.slice(0, separatorIndex).trim(),
    model: value.slice(separatorIndex + 2).trim()
  };
}

function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}

function shouldRetryWithRefresh(path: string, options: RequestOptions, error: unknown): boolean {
  return (
    error instanceof APIRequestError &&
    error.status === 401 &&
    Boolean(options.token) &&
    !path.startsWith("/auth/")
  );
}

function errorMessage(value: unknown, fallback: string): string {
  if (value instanceof Error) {
    return value.message;
  }

  return fallback;
}
