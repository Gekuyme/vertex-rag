# Third-Party Notices

Vertex RAG is distributed under the MIT License, but it depends on third-party software with their own licenses and notices.

This file is a practical notice for repository users and contributors. It is not intended to be a complete legal inventory of every transitive dependency across every platform and build environment.

## Main dependency groups

The project uses third-party libraries from the JavaScript/TypeScript, Go, and infrastructure ecosystems, including but not limited to:

- Next.js, React, and related frontend tooling
- Playwright for end-to-end testing
- AWS SDK for Go
- pgx for PostgreSQL access
- asynq for background job processing
- Redis, PostgreSQL, MinIO, and Ollama container images in local/self-hosted setups

Each dependency remains subject to its own license terms.

## Notable transitive dependencies

### sharp / libvips

The web application may use the Next.js image pipeline, which can transitively rely on `sharp` and prebuilt `libvips` binaries depending on platform and install path.

Some `libvips` binary packages are distributed under `LGPL-3.0-or-later`, and some related packages use mixed licensing expressions such as:

- `LGPL-3.0-or-later`
- `Apache-2.0 AND LGPL-3.0-or-later`
- `Apache-2.0 AND LGPL-3.0-or-later AND MIT`

Repository users distributing builds that include these binaries should review the corresponding package metadata and upstream license terms.

Upstream references:

- [sharp](https://github.com/lovell/sharp)
- [libvips](https://github.com/libvips/libvips)

### caniuse-lite

The frontend dependency tree also includes `caniuse-lite`, which is commonly used by frontend tooling and is published under `CC-BY-4.0`.

Upstream reference:

- [caniuse-lite](https://github.com/browserslist/caniuse-lite)

## Fonts and external assets

The web app currently imports the `Inter` font from Google Fonts in the frontend stylesheet.

Users with stricter privacy, compliance, or redistribution requirements may prefer to self-host fonts and review the relevant upstream font licensing and delivery terms.

## Container images

Local and self-hosted deployment uses third-party container images, including:

- `pgvector/pgvector`
- `redis`
- `minio/minio`
- `ollama/ollama`

These images are distributed under their own terms and may include additional third-party components.

## Contributor guidance

When adding a new dependency, try to prefer permissive licenses where practical and avoid introducing unusual licensing terms without documenting them clearly.

If a new dependency has licensing implications that are likely to matter to downstream users, update this file.
