import type { Locale } from "./config";

const dictionaries = {
  ru: {
    metadata: {
      title: "Vertex RAG",
      description: "MVP shell для платформы Vertex RAG"
    },
    localeLabel: "Русский",
    localeSwitcherLabel: "Язык интерфейса",
    auth: {
      loginTitle: "Вход",
      ownerConsole: "Консоль владельца и администратора",
      loginTab: "Войти",
      registerTab: "Регистрация владельца",
      organization: "Организация",
      organizationPlaceholder: "ООО Пример",
      email: "Почта",
      password: "Пароль",
      submitBusy: "Обработка...",
      submitLogin: "Войти",
      submitRegister: "Создать организацию"
    },
    shell: {
      workspace: "Защищённое рабочее пространство",
      chats: "Чаты",
      createChat: "Создать чат",
      noChats: "Пока нет чатов.",
      settings: "Настройки",
      openSettings: "Открыть настройки",
      knowledge: "База знаний",
      users: "Пользователи",
      account: "Аккаунт",
      newChat: "Новый чат",
      deleteChat: "Удалить чат",
      renameChat: "Изменить название",
      chatActions: "Действия с чатом",
      toggleNavigation: "Открыть навигацию"
    },
    chat: {
      emptyTitle: "Задайте вопрос",
      emptyBody: "Ассистент отвечает, используя базу знаний вашей организации.",
      emptyGreetings: [
        (name: string) => `Чем помочь, ${name}?`,
        (name: string) => `Привет, ${name}. С чего начнем?`,
        (name: string) => `Над чем вы работаете, ${name}?`,
        (name: string) => `Готов погрузиться, ${name}. Что разбираем?`,
        (name: string) => `Какую задачу решаем сегодня, ${name}?`,
        (name: string) => `Что хотите выяснить, ${name}?`
      ],
      you: "Вы",
      assistant: "Ассистент",
      failedDelivery: "Сообщение не доставлено. Исправьте причину ошибки и отправьте заново.",
      noKnowledge: "Недостаточно данных в базе знаний.",
      noKnowledgeHint:
        "Нет релевантных источников в базе знаний для этой роли. Проверьте доступ к документу (allowed roles) или загрузите документ с нужными правами.",
      sources: "Источники",
      composerPlaceholder: "Ask anything",
      composerOptions: "Параметры сообщения",
      uploadFile: "Загрузить файл",
      model: "Модель",
      modelQwen: "Qwen",
      modelGemini: "Gemini",
      responseMode: "Режим ответа",
      responseProfile: "Профиль ответа",
      unstrictDisabled: "Роль не разрешает unstrict",
      modeStrict: "Строгий",
      modeUnstrict: "Нестрогий",
      streamRetrievingFast: "Быстро ищет контекст",
      streamRetrieving: "Ищет релевантный контекст",
      streamDraftingThinking: "Собирает и сверяет ответ",
      streamDrafting: "Готовит ответ",
      streamFinalizing: "Финализирует ответ",
      streamThinkingDeep: "Думает глубже",
      streamThinking: "Думает"
    },
    responseProfiles: {
      fast: "Fast",
      balanced: "Balanced",
      thinking: "Thinking"
    },
    knowledge: {
      readOnlyNotice: "Вы можете просматривать документы, но у вас нет прав на загрузку.",
      upload: "Загрузка",
      titleOptional: "Название (необязательно)",
      titlePlaceholder: "Регламент компании",
      file: "Файл",
      allowedRoles: "Разрешённые роли",
      uploadBusy: "Загрузка...",
      uploadSubmit: "Загрузить документ",
      documents: "Документы",
      reingestAll: "Переиндексировать все",
      name: "Название",
      filename: "Файл",
      status: "Статус",
      accessRoles: "Роли доступа",
      actions: "Действия",
      reingest: "Переиндексировать",
      noDocuments: "Документов пока нет."
    },
    users: {
      noAccess: "У текущего пользователя нет прав на управление пользователями.",
      title: "Пользователи и роли",
      save: "Сохранить",
      notFound: "Пользователи не найдены."
    },
    roles: {
      noAccess: "У текущего пользователя нет прав на управление ролями.",
      title: "Роли и разрешения",
      roleName: "Название роли",
      rolePlaceholder: "Например: Analyst",
      permissions: "Разрешения",
      update: "Обновить роль",
      create: "Создать роль",
      cancel: "Отмена",
      role: "Роль",
      edit: "Редактировать",
      delete: "Удалить",
      notFound: "Роли не найдены.",
      defaultSuffix: "default",
      permissionUploadDocs: "Загрузка документов",
      permissionManageUsers: "Управление пользователями",
      permissionManageRoles: "Управление ролями",
      permissionManageDocuments: "Управление документами",
      permissionToggleWebSearch: "Web-search в unstrict",
      permissionUseUnstrict: "Использование unstrict"
    },
    account: {
      title: "Аккаунт",
      user: "Пользователь",
      role: "Роль",
      organization: "Организация",
      defaultMode: "Режим по умолчанию",
      defaultModeHint: "Определяет режим, если вы не указываете его в сообщении."
    },
    settings: {
      title: "Настройки",
      close: "Закрыть настройки",
      knowledge: "База знаний",
      users: "Пользователи",
      roles: "Роли",
      account: "Аккаунт",
      logout: "Выйти",
      appearance: "Оформление",
      theme: "Тема",
      themeHint: "Выберите системную, светлую или тёмную тему интерфейса.",
      themeSystem: "System",
      themeLight: "Light",
      themeDark: "Dark",
      documentLibrary: "Библиотека документов",
      roleDirectory: "Каталог ролей",
      manageUsersHint: "Меняйте роли пользователей прямо из настроек.",
      manageRolesHint: "Создавайте и редактируйте роли без отдельной админ-панели.",
      knowledgeHint: "Просматривайте документы и запускайте переиндексацию без перехода на отдельный экран.",
      accountHint: "Персональные и поведенческие настройки текущего аккаунта.",
      saveChanges: "Сохранить изменения",
      createRole: "Создать роль",
      noRolesSelected: "Нет выбранных ролей",
      noPermissions: "Нет разрешений",
      uploadAllowedRoles: "Разрешённые роли",
      themeActive: (value: string) => `Активно: ${value}`
    },
    uploadModal: {
      title: "Загрузка документа",
      close: "Закрыть окно загрузки",
      noAccess: "У текущего пользователя нет прав на загрузку.",
      selectFile: "Выберите файл для загрузки.",
      selectRole: "Выберите хотя бы одну роль доступа."
    },
    sourcePreview: {
      title: "Источник ответа",
      close: "Закрыть окно источника",
      open: "Просмотр источника",
      document: "Документ",
      file: "Файл",
      page: "Страница",
      section: "Секция",
      snippet: "Фрагмент",
      snippetMissing: "Фрагмент не указан",
      scoring: "Скоринг",
      documentStatus: "Статус документа",
      goToKnowledge: "Перейти к базе знаний",
      unknown: "неизвестно"
    },
    onboarding: {
      title: "Онбординг",
      skip: "Пропустить",
      previous: "Назад",
      done: "Готово",
      next: "Далее",
      step: (current: number, total: number) => `Шаг ${current} из ${total}`,
      steps: [
        {
          title: "Навигация по чатам",
          description: "Здесь список ваших диалогов. Переключайтесь между ними одним кликом."
        },
        {
          title: "Новый чат",
          description: "Нажмите плюс, чтобы быстро создать новый диалог."
        },
        {
          title: "Настройки",
          description: "В настройках доступны аккаунт, роли, база знаний и выход."
        },
        {
          title: "Загрузка файла",
          description: "Через эту кнопку открывается окно загрузки документа в базу знаний."
        },
        {
          title: "Режим ответа",
          description: "Выбирайте Строгий или Нестрогий режим ответа для текущего сообщения."
        }
      ]
    },
    status: {
      uploaded: "Загружен",
      processing: "Обрабатывается",
      indexing: "Индексируется",
      ready: "Готов",
      failed: "Ошибка",
      active: "Активен",
      invited: "Приглашён",
      disabled: "Отключён",
      unknown: "Неизвестно"
    },
    feedback: {
      signedIn: (email: string) => `Вход выполнен как ${email}`,
      sessionRestored: (email: string) => `Сессия восстановлена для ${email}`,
      organizationCreated: (email: string) => `Организация создана. Вход выполнен как ${email}.`,
      userRoleUpdated: "Роль пользователя обновлена.",
      documentUploaded: "Документ загружен и отправлен в индексацию.",
      documentReingest: "Документ отправлен в переиндексацию.",
      allDocumentsReingest: (scheduled: number, total: number) => `Переиндексация запланирована: ${scheduled}/${total}.`,
      newChatCreated: "Новый чат создан.",
      chatRenamed: "Название чата обновлено.",
      requestFailed: (status: number) => `Ошибка запроса: ${status}`,
      responseMissing: "Не удалось получить ответ.",
      chatDeleted: "Чат удалён.",
      defaultModeUpdated: "Режим по умолчанию обновлён.",
      roleNameRequired: "Название роли обязательно.",
      systemRoleDelete: "Системные роли нельзя удалить.",
      roleUpdated: "Роль обновлена.",
      roleCreated: "Роль создана.",
      roleDeleted: "Роль удалена.",
      loggedOut: "Вы вышли из аккаунта.",
      unexpectedError: "Непредвиденная ошибка запроса",
      deleteChatConfirm: "Удалить текущий чат? Это действие нельзя отменить.",
      deleteRoleConfirm: (name: string) => `Удалить роль «${name}»?`
    }
  },
  en: {
    metadata: {
      title: "Vertex RAG",
      description: "MVP shell for the Vertex RAG platform"
    },
    localeLabel: "English",
    localeSwitcherLabel: "Interface language",
    auth: {
      loginTitle: "Sign in",
      ownerConsole: "Owner and administrator console",
      loginTab: "Sign in",
      registerTab: "Owner signup",
      organization: "Organization",
      organizationPlaceholder: "Acme Inc.",
      email: "Email",
      password: "Password",
      submitBusy: "Processing...",
      submitLogin: "Sign in",
      submitRegister: "Create organization"
    },
    shell: {
      workspace: "Secure workspace",
      chats: "Chats",
      createChat: "Create chat",
      noChats: "No chats yet.",
      settings: "Settings",
      openSettings: "Open settings",
      knowledge: "Knowledge base",
      users: "Users",
      account: "Account",
      newChat: "New chat",
      deleteChat: "Delete chat",
      renameChat: "Rename chat",
      chatActions: "Chat actions",
      toggleNavigation: "Toggle navigation"
    },
    chat: {
      emptyTitle: "Ask a question",
      emptyBody: "The assistant answers using your organization's knowledge base.",
      emptyGreetings: [
        (name: string) => `How can I help, ${name}?`,
        (name: string) => `Hey, ${name}. Ready to dive in?`,
        (name: string) => `What are you working on, ${name}?`,
        (name: string) => `What can I help you think through, ${name}?`,
        (name: string) => `Where do you want to start, ${name}?`,
        (name: string) => `What are we tackling today, ${name}?`
      ],
      you: "You",
      assistant: "Assistant",
      failedDelivery: "Message was not delivered. Fix the error and send it again.",
      noKnowledge: "Not enough data in the knowledge base.",
      noKnowledgeHint:
        "No relevant knowledge-base sources are available for this role. Check document access (allowed roles) or upload a document with the required permissions.",
      sources: "Sources",
      composerPlaceholder: "Ask anything",
      composerOptions: "Message options",
      uploadFile: "Upload file",
      model: "Model",
      modelQwen: "Qwen",
      modelGemini: "Gemini",
      responseMode: "Response mode",
      responseProfile: "Response profile",
      unstrictDisabled: "This role cannot use unstrict",
      modeStrict: "Strict",
      modeUnstrict: "Unstrict",
      streamRetrievingFast: "Quickly finding context",
      streamRetrieving: "Finding relevant context",
      streamDraftingThinking: "Assembling and checking the answer",
      streamDrafting: "Drafting the answer",
      streamFinalizing: "Finalizing the answer",
      streamThinkingDeep: "Thinking more deeply",
      streamThinking: "Thinking"
    },
    responseProfiles: {
      fast: "Fast",
      balanced: "Balanced",
      thinking: "Thinking"
    },
    knowledge: {
      readOnlyNotice: "You can view documents, but you do not have permission to upload them.",
      upload: "Upload",
      titleOptional: "Title (optional)",
      titlePlaceholder: "Company policy",
      file: "File",
      allowedRoles: "Allowed roles",
      uploadBusy: "Uploading...",
      uploadSubmit: "Upload document",
      documents: "Documents",
      reingestAll: "Reindex all",
      name: "Title",
      filename: "File",
      status: "Status",
      accessRoles: "Access roles",
      actions: "Actions",
      reingest: "Reindex",
      noDocuments: "No documents yet."
    },
    users: {
      noAccess: "The current user does not have permission to manage users.",
      title: "Users and roles",
      save: "Save",
      notFound: "No users found."
    },
    roles: {
      noAccess: "The current user does not have permission to manage roles.",
      title: "Roles and permissions",
      roleName: "Role name",
      rolePlaceholder: "For example: Analyst",
      permissions: "Permissions",
      update: "Update role",
      create: "Create role",
      cancel: "Cancel",
      role: "Role",
      edit: "Edit",
      delete: "Delete",
      notFound: "No roles found.",
      defaultSuffix: "default",
      permissionUploadDocs: "Upload documents",
      permissionManageUsers: "Manage users",
      permissionManageRoles: "Manage roles",
      permissionManageDocuments: "Manage documents",
      permissionToggleWebSearch: "Web search in unstrict",
      permissionUseUnstrict: "Use unstrict"
    },
    account: {
      title: "Account",
      user: "User",
      role: "Role",
      organization: "Organization",
      defaultMode: "Default mode",
      defaultModeHint: "Defines the mode when you do not specify one in the message."
    },
    settings: {
      title: "Settings",
      close: "Close settings",
      knowledge: "Knowledge base",
      users: "Users",
      roles: "Roles",
      account: "Account",
      logout: "Log out",
      appearance: "Appearance",
      theme: "Theme",
      themeHint: "Choose the system, light, or dark interface theme.",
      themeSystem: "System",
      themeLight: "Light",
      themeDark: "Dark",
      documentLibrary: "Document library",
      roleDirectory: "Role directory",
      manageUsersHint: "Update user roles directly from settings.",
      manageRolesHint: "Create and edit roles without a separate admin screen.",
      knowledgeHint: "Review documents and trigger reindexing without leaving settings.",
      accountHint: "Personal and behavior settings for the current account.",
      saveChanges: "Save changes",
      createRole: "Create role",
      noRolesSelected: "No roles selected",
      noPermissions: "No permissions",
      uploadAllowedRoles: "Allowed roles",
      themeActive: (value: string) => `Active: ${value}`
    },
    uploadModal: {
      title: "Upload document",
      close: "Close upload dialog",
      noAccess: "The current user does not have upload permission.",
      selectFile: "Select a file to upload.",
      selectRole: "Select at least one access role."
    },
    sourcePreview: {
      title: "Answer source",
      close: "Close source dialog",
      open: "Source preview",
      document: "Document",
      file: "File",
      page: "Page",
      section: "Section",
      snippet: "Snippet",
      snippetMissing: "Snippet is not available",
      scoring: "Scoring",
      documentStatus: "Document status",
      goToKnowledge: "Open knowledge base",
      unknown: "unknown"
    },
    onboarding: {
      title: "Onboarding",
      skip: "Skip",
      previous: "Back",
      done: "Done",
      next: "Next",
      step: (current: number, total: number) => `Step ${current} of ${total}`,
      steps: [
        {
          title: "Chat navigation",
          description: "This is the list of your conversations. Switch between them in one click."
        },
        {
          title: "New chat",
          description: "Click plus to quickly create a new conversation."
        },
        {
          title: "Settings",
          description: "Settings include account, roles, knowledge base, and sign out."
        },
        {
          title: "File upload",
          description: "This button opens the upload dialog for knowledge-base documents."
        },
        {
          title: "Response mode",
          description: "Choose Strict or Unstrict response mode for the current message."
        }
      ]
    },
    status: {
      uploaded: "Uploaded",
      processing: "Processing",
      indexing: "Indexing",
      ready: "Ready",
      failed: "Failed",
      active: "Active",
      invited: "Invited",
      disabled: "Disabled",
      unknown: "Unknown"
    },
    feedback: {
      signedIn: (email: string) => `Signed in as ${email}`,
      sessionRestored: (email: string) => `Session restored for ${email}`,
      organizationCreated: (email: string) => `Organization created. Signed in as ${email}.`,
      userRoleUpdated: "User role updated.",
      documentUploaded: "Document uploaded and sent for indexing.",
      documentReingest: "Document sent for reindexing.",
      allDocumentsReingest: (scheduled: number, total: number) => `Reindex scheduled: ${scheduled}/${total}.`,
      newChatCreated: "New chat created.",
      chatRenamed: "Chat renamed.",
      requestFailed: (status: number) => `Request failed: ${status}`,
      responseMissing: "Failed to get a response.",
      chatDeleted: "Chat deleted.",
      defaultModeUpdated: "Default mode updated.",
      roleNameRequired: "Role name is required.",
      systemRoleDelete: "System roles cannot be deleted.",
      roleUpdated: "Role updated.",
      roleCreated: "Role created.",
      roleDeleted: "Role deleted.",
      loggedOut: "Signed out.",
      unexpectedError: "Unexpected request error",
      deleteChatConfirm: "Delete the current chat? This action cannot be undone.",
      deleteRoleConfirm: (name: string) => `Delete role "${name}"?`
    }
  }
} as const;

export type Messages = (typeof dictionaries)[Locale];

export function getMessages(locale: Locale): Messages {
  return dictionaries[locale];
}
