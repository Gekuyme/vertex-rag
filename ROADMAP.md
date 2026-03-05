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
- [x] Owner/Admin UI: управление пользователями (назначение ролей).
- [x] **Критерий готовности:** пользователь без прав не может дергать admin endpoints и не видит чужой `org_id`.

## Milestone 2 — Knowledge Base: документы + ACL по ролям
- [x] Схема БД: `documents`, `document_chunks` (embedding + metadata + `allowed_role_ids int[]`).
- [x] API загрузки документа: `POST /documents/upload` (multipart) + выбор `allowed_role_ids[]`.
- [x] S3 storage через MinIO: загрузка файла, сохранение `storage_key`.
- [x] API списка документов: `GET /documents` + статусы обработки (`uploaded|processing|ready|failed`).
- [x] UI `/knowledge`: загрузка, выбор ролей доступа, просмотр статуса.
- [x] **Критерий готовности:** документ, доступный только Owner, не появляется в retrieval для Member.

## Milestone 3 — Ingestion pipeline (worker) + индексация
- [x] Очередь задач (Redis): задача `ingest_document(document_id)`.
- [x] Extract text:
  - [x] PDF: `pdftotext` в контейнере worker.
  - [x] DOCX: извлечение текста библиотекой.
  - [x] MD/TXT: напрямую.
- [x] Нормализация текста (удаление мусора/повторов).
- [x] Чанкинг: ~800–1200 токенов, overlap ~100, metadata (`page/section/source`).
- [x] Embeddings provider interface: `Embed(texts[]) -> vectors[]` (batch + rate limit).
- [x] Реализации embeddings: `openai`, `ollama`.
- [x] Запись чанков в `document_chunks` + индексы (pgvector + full-text).
- [x] Инвалидация кэша: `organizations.kb_version++` при успешной индексации/изменении KB.
- [x] **Критерий готовности:** загрузка PDF → `ready` → чанки есть в БД, embedding не пустой.

## Milestone 4 — RAG retrieval (hybrid) + citations
- [x] Retrieval: vector similarity + full-text rank с объединенным скорингом.
- [x] Фильтры retrieval: `org_id`, `allowed_role_ids`, `documents.status=ready`.
- [x] Формирование “контекста” для LLM: top-k чанков + короткие snippets.
- [x] Структура citations: `chunk_id`, `document_id`, `doc_title`, `snippet`, `page/section`.
- [x] Endpoint/метод для дебага: вернуть retrieval результаты (только admin/dev) для диагностики качества.
- [x] **Критерий готовности:** на один и тот же вопрос retrieval стабильно возвращает релевантные чанки.

## Milestone 5 — Strict / Unstrict (надежность переключателя)
- [x] UI toggle `strict|unstrict` в header.
- [x] Сохранение default режима: `PATCH /me/settings { default_mode }`.
- [x] Сервер фиксирует mode на старте запроса (per-request) и сохраняет в `messages.mode`.
- [x] Strict:
  - [x] Запрет web-search/tool вызовов.
  - [x] Ответ только на основе retrieved chunks.
  - [x] Требовать structured output: `answer + citations[]`.
  - [x] Если данных нет/слабые → “Недостаточно данных в базе знаний” (без додумывания).
  - [x] Server-side guard: если citations невалидны → 1 retry → fallback.
- [x] Unstrict:
  - [x] Может отвечать общими знаниями.
  - [x] RBAC на внутренние чанки сохраняется.
  - [x] Web-search модуль (опционально, feature-flag).
- [x] **Критерий готовности:** переключение в UI не влияет на in-flight стрим, влияет только на следующий запрос.

## Quality pass — Ingestion + answering (качество/надежность)
- [x] PDF: извлечение `pdftotext` с UTF-8 + page metadata из `\f` разрывов.
- [x] Markdown: section metadata по ближайшему `#...` heading (для retrieval/citations).
- [x] Нормализация текста: сохранить списки/таблицы/кодовые блоки, склейка переносов и `exam-\nple`.
- [x] Чанкинг: предпочтение границ абзацев (`\n\n`) при разрезе.
- [x] DOCX: сбор текста по параграфам (сохранить структуру, без “newline per run”).
- [x] Strict guard: проверять прямые цитаты по полному тексту retrieved chunk (не по усеченному snippet).
- [x] Unstrict: поддержка ссылок на web-контекст через `[Wn]` (например: `[W1]`).
- [x] Embeddings: ускорение Ollama через параллельные запросы + embedding input с `Document/Section/Page`.
- [x] Regression: `make test-integration` проходит.

## Stabilization pass — RAG/Strict/Unstrict hardening
- [x] Backfill прав `can_use_unstrict` для существующих ролей и временная legacy-совместимость через `UNSTRICT_LEGACY_TOGGLE_WEB_SEARCH`.
- [x] UI больше не скрывает ошибку `unstrict`: сообщение пользователя остается в чате, а причина показывается inline.
- [x] Retrieval стал детерминированным: стабильные tie-breakers, candidate diversification per document и safe filter по `vector_dims(...)`.
- [x] `/admin/retrieval/debug` использует тот же query-building, что и production chat, и возвращает `embed_query` + `text_query`.
- [x] Strict больше не маскирует LLM transport/empty-output ошибки под fallback “Недостаточно данных...”.
- [x] Нормализация soft hyphen / zero-width артефактов добавлена в ingestion и strict quote matching.
- [x] Smoke-сценарий проверяет частый термин (`что такое строка`) и regression для `unstrict` RBAC.

## Milestone 6 — LLM Provider layer (сменность провайдера)
- [x] Интерфейс `LLMProvider` (stream + non-stream).
- [x] Реализации: `openai`, `ollama`.
- [x] Конфиг выбора провайдера: `LLM_PROVIDER`, `EMBED_PROVIDER`.
- [x] Настроить timeouts, retries, backoff, лимиты контекста.
- [x] **Критерий готовности:** можно переключить провайдера env-переменной без изменений кода приложения.

## Milestone 7 — Chat UI (AI-like) + streaming
- [x] Схема БД: `chats`, `messages`.
- [x] API чатов: list/create/get/delete.
- [x] API сообщений: `POST /chats/:id/messages/stream` (SSE).
- [x] UI `/chat`:
  - [x] Sidebar список чатов + “New chat”.
  - [x] Streaming ответа (“typing”).
  - [x] Отображение режима (strict/unstrict) на сообщениях ассистента.
  - [x] Отображение citations (кликабельные) + preview chunk/doc.
- [x] **Критерий готовности:** можно создать чат, задать вопрос, увидеть стрим + citations (в strict при наличии данных).

## Milestone 8 — Кэширование (Redis) + производительность
- [x] Кэш retrieval: ключ включает `org_id`, `role_id`, `mode`, `normalized_query`, `kb_version`.
- [x] Кэш answer:
  - [x] strict: кэшировать ответ + citations.
  - [x] unstrict: опционально (в MVP можно включить, но осторожно с web-search).
- [x] Pre-warm/часто выбираемые документы: статистика top docs (минимум счетчики).
- [x] **Критерий готовности:** повторный вопрос отвечает быстрее за счет кэша, без нарушения RBAC.

## Milestone 9 — Admin: роли/доступы (Owner UI)
- [x] UI `/admin/roles`: CRUD ролей + permissions.
- [x] UI назначения ролей пользователям.
- [x] Валидация: нельзя удалить роль, если она назначена пользователям (или предусмотреть миграцию).
- [x] **Критерий готовности:** Owner может создать роль, загрузить документ только для этой роли, и проверить доступ.

## Milestone 10 — Self-host release (без исходников)
- [x] `deploy/compose/docker-compose.yml` использует prebuilt images из private registry.
- [x] Документация деплоя: требования, env-переменные, включение `ollama`, миграции БД.
- [x] Hardening минимум: секреты, CORS, cookie settings, rate limiting.
- [x] **Критерий готовности:** поднятие через compose “с нуля” и работа end-to-end.

---

## Тесты и приемка (минимум для MVP)
- [x] Unit (Go):
  - [x] RBAC фильтры retrieval.
  - [x] Strict guard (citations валидны).
  - [x] Cache key включает `kb_version`.
- [x] Integration (compose):
  - [x] Upload → ingest → strict answer с citations.
  - [x] Документ restricted → недоступен пользователю без роли.
  - [x] Strict/unstrict toggle влияет только на следующий запрос.
- [x] E2E (Playwright минимум):
  - [x] Login → upload → chat strict → citations.

---

## Definition of Done (MVP)
- [x] Рабочий чат со стримингом + toggle strict/unstrict.
- [x] UI загрузки документов с выбором ролей доступа.
- [x] Strict отвечает только на основе KB, с цитатами, без галлюцинаций (fallback “Недостаточно данных…”).
- [x] RBAC гарантирует отсутствие утечек знаний между ролями и org.
- [x] Self-host поднимается через docker-compose с закрытыми Docker images.
