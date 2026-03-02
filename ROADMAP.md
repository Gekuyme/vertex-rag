# Vertex RAG — MVP Roadmap (Checklist)

Цель: MVP RAG-системы для бизнеса с современным AI chat UI, загрузкой документов в knowledge base, RAG-ответами с контролем доступа по ролям, и надежным переключателем `strict / unstrict`.

## Принятые решения для MVP (зафиксировано)
- [x] Подтвердить, что MVP = **SaaS-ready + self-host** (одна кодовая база, мульти‑тенант) и self-host поставляется как **Docker images** без исходников.
- [x] Стек: **Next.js (frontend) + Go (backend/worker)**.
- [x] Vector store: **Postgres + pgvector**.
- [x] Очередь и кэш: **Redis**.
- [x] Storage: **S3 (MinIO для dev/self-host)**.
- [x] LLM/Embeddings: **сменные провайдеры** (OpenAI API и локальный **Ollama**).
- [x] Unstrict web-search: **опционально**, по умолчанию выключено, через **Search API**.
- [x] Структура: **только чаты** (без проектов в MVP).
- [x] ACL: **по ролям** (без per-user исключений).

---

## Milestone 0 — Репо и инфраструктура разработки
- [x] Инициализировать монорепо структуру: `apps/web`, `apps/api`, `apps/worker`, `deploy/compose`, `db/migrations`, `scripts`.
- [x] Подготовить `docker-compose` для dev/self-host: `web`, `api`, `worker`, `postgres`, `redis`, `minio`, опционально `ollama`.
- [x] Настроить конфиги через env: `.env.example` с пояснениями переменных.
- [x] Добавить миграции Postgres + расширения: `vector`, `pg_trgm`.
- [x] Добавить базовые health endpoints: `GET /healthz` (api/worker).

## Milestone 1 — Auth + Org + RBAC (ядро безопасности)
- [x] Схема БД: `organizations`, `users`, `roles` (везде `org_id` где нужно).
- [x] Seed default ролей: `Owner`, `Admin`, `Member`, `Viewer`.
- [x] Auth (MVP): email+password (Argon2id), JWT access + refresh (httpOnly cookie).
- [x] Middleware: извлечение `org_id` + роль пользователя для каждого запроса.
- [x] Permissions (минимум): `can_upload_docs`, `can_manage_users`, `can_manage_roles`, `can_manage_documents`, `can_toggle_web_search`.
- [ ] Owner/Admin UI: управление пользователями (назначение ролей).
- [x] **Критерий готовности:** пользователь без прав не может дергать admin endpoints и не видит чужой `org_id`.

## Milestone 2 — Knowledge Base: документы + ACL по ролям
- [ ] Схема БД: `documents`, `document_chunks` (embedding + metadata + `allowed_role_ids int[]`).
- [ ] API загрузки документа: `POST /documents/upload` (multipart) + выбор `allowed_role_ids[]`.
- [ ] S3 storage через MinIO: загрузка файла, сохранение `storage_key`.
- [ ] API списка документов: `GET /documents` + статусы обработки (`uploaded|processing|ready|failed`).
- [ ] UI `/knowledge`: загрузка, выбор ролей доступа, просмотр статуса.
- [ ] **Критерий готовности:** документ, доступный только Owner, не появляется в retrieval для Member.

## Milestone 3 — Ingestion pipeline (worker) + индексация
- [ ] Очередь задач (Redis): задача `ingest_document(document_id)`.
- [ ] Extract text:
  - [ ] PDF: `pdftotext` в контейнере worker.
  - [ ] DOCX: извлечение текста библиотекой.
  - [ ] MD/TXT: напрямую.
- [ ] Нормализация текста (удаление мусора/повторов).
- [ ] Чанкинг: ~800–1200 токенов, overlap ~100, metadata (`page/section/source`).
- [ ] Embeddings provider interface: `Embed(texts[]) -> vectors[]` (batch + rate limit).
- [ ] Реализации embeddings: `openai`, `ollama`.
- [ ] Запись чанков в `document_chunks` + индексы (pgvector + full-text).
- [ ] Инвалидация кэша: `organizations.kb_version++` при успешной индексации/изменении KB.
- [ ] **Критерий готовности:** загрузка PDF → `ready` → чанки есть в БД, embedding не пустой.

## Milestone 4 — RAG retrieval (hybrid) + citations
- [ ] Retrieval: vector similarity + full-text rank с объединенным скорингом.
- [ ] Фильтры retrieval: `org_id`, `allowed_role_ids`, `documents.status=ready`.
- [ ] Формирование “контекста” для LLM: top-k чанков + короткие snippets.
- [ ] Структура citations: `chunk_id`, `document_id`, `doc_title`, `snippet`, `page/section`.
- [ ] Endpoint/метод для дебага: вернуть retrieval результаты (только admin/dev) для диагностики качества.
- [ ] **Критерий готовности:** на один и тот же вопрос retrieval стабильно возвращает релевантные чанки.

## Milestone 5 — Strict / Unstrict (надежность переключателя)
- [ ] UI toggle `strict|unstrict` в header.
- [ ] Сохранение default режима: `PATCH /me/settings { default_mode }`.
- [ ] Сервер фиксирует mode на старте запроса (per-request) и сохраняет в `messages.mode`.
- [ ] Strict:
  - [ ] Запрет web-search/tool вызовов.
  - [ ] Ответ только на основе retrieved chunks.
  - [ ] Требовать structured output: `answer + citations[]`.
  - [ ] Если данных нет/слабые → “Недостаточно данных в базе знаний” (без додумывания).
  - [ ] Server-side guard: если citations невалидны → 1 retry → fallback.
- [ ] Unstrict:
  - [ ] Может отвечать общими знаниями.
  - [ ] RBAC на внутренние чанки сохраняется.
  - [ ] Web-search модуль (опционально, feature-flag).
- [ ] **Критерий готовности:** переключение в UI не влияет на in-flight стрим, влияет только на следующий запрос.

## Milestone 6 — LLM Provider layer (сменность провайдера)
- [ ] Интерфейс `LLMProvider` (stream + non-stream).
- [ ] Реализации: `openai`, `ollama`.
- [ ] Конфиг выбора провайдера: `LLM_PROVIDER`, `EMBED_PROVIDER`.
- [ ] Настроить timeouts, retries, backoff, лимиты контекста.
- [ ] **Критерий готовности:** можно переключить провайдера env-переменной без изменений кода приложения.

## Milestone 7 — Chat UI (AI-like) + streaming
- [ ] Схема БД: `chats`, `messages`.
- [ ] API чатов: list/create/get/delete.
- [ ] API сообщений: `POST /chats/:id/messages/stream` (SSE).
- [ ] UI `/chat`:
  - [ ] Sidebar список чатов + “New chat”.
  - [ ] Streaming ответа (“typing”).
  - [ ] Отображение режима (strict/unstrict) на сообщениях ассистента.
  - [ ] Отображение citations (кликабельные) + preview chunk/doc.
- [ ] **Критерий готовности:** можно создать чат, задать вопрос, увидеть стрим + citations (в strict при наличии данных).

## Milestone 8 — Кэширование (Redis) + производительность
- [ ] Кэш retrieval: ключ включает `org_id`, `role_id`, `mode`, `normalized_query`, `kb_version`.
- [ ] Кэш answer:
  - [ ] strict: кэшировать ответ + citations.
  - [ ] unstrict: опционально (в MVP можно включить, но осторожно с web-search).
- [ ] Pre-warm/часто выбираемые документы: статистика top docs (минимум счетчики).
- [ ] **Критерий готовности:** повторный вопрос отвечает быстрее за счет кэша, без нарушения RBAC.

## Milestone 9 — Admin: роли/доступы (Owner UI)
- [ ] UI `/admin/roles`: CRUD ролей + permissions.
- [ ] UI назначения ролей пользователям.
- [ ] Валидация: нельзя удалить роль, если она назначена пользователям (или предусмотреть миграцию).
- [ ] **Критерий готовности:** Owner может создать роль, загрузить документ только для этой роли, и проверить доступ.

## Milestone 10 — Self-host release (без исходников)
- [ ] `deploy/compose/docker-compose.yml` использует prebuilt images из private registry.
- [ ] Документация деплоя: требования, env-переменные, включение `ollama`, миграции БД.
- [ ] Hardening минимум: секреты, CORS, cookie settings, rate limiting.
- [ ] **Критерий готовности:** поднятие через compose “с нуля” и работа end-to-end.

---

## Тесты и приемка (минимум для MVP)
- [ ] Unit (Go):
  - [ ] RBAC фильтры retrieval.
  - [ ] Strict guard (citations валидны).
  - [ ] Cache key включает `kb_version`.
- [ ] Integration (compose):
  - [ ] Upload → ingest → strict answer с citations.
  - [ ] Документ restricted → недоступен пользователю без роли.
  - [ ] Strict/unstrict toggle влияет только на следующий запрос.
- [ ] E2E (Playwright минимум):
  - [ ] Login → upload → chat strict → citations.

---

## Definition of Done (MVP)
- [ ] Рабочий чат со стримингом + toggle strict/unstrict.
- [ ] UI загрузки документов с выбором ролей доступа.
- [ ] Strict отвечает только на основе KB, с цитатами, без галлюцинаций (fallback “Недостаточно данных…”).
- [ ] RBAC гарантирует отсутствие утечек знаний между ролями и org.
- [ ] Self-host поднимается через docker-compose с закрытыми Docker images.
