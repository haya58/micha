# Micha

<p align="center">
  <img src="docs/images/micha_logo.png" alt="Micha" width="600">
</p>

## Overview
Micha (pronounced "Mika") is an LLM chat bot application that enlivens Discord communities.
It is simple and robust, while providing the features expected of a modern LLM chat bot.

Why Micha ?
→ Inspired by [Micha Jawkân-chan](https://www.symphogear-gx.com/characters/micha.php)

## Features
Micha can do the following.
### Chatting with Micha
- Supports reading uploaded images as well as text chat
- Channels where the bot can talk or read can be restricted by category
- Persona and instructions configurable in Markdown format
- Supports simple shell command execution
  - Spawns a disposable Docker container per execution
- Periodically patrols read-only channels and reflects topics
- Maintains advanced long-term memory integrated with a vectorized DB
- Supports web search and fetching text and images from X (Twitter) links
### Interactions with Micha
- Interaction features
  - Spontaneous messages from the bot
  - Random interactions with emoji
### OpenAI-compatible API support
- Verified APIs
  - Genuine OpenAI API
  - OpenCode Go API
- Multiple API endpoints can be registered
  - For example, use the OpenCode Go API for text chat while using the genuine OpenAI API only for Embedding
### Container-first design
- Docker image provided
- Native support for deployment to Kubernetes

## Quickstart
### Prerequisites
- Create an application in the Discord Developer Portal and obtain a bot token
  - Please enable Privileged Intent
- If you want to enable search, obtain a Brave API key

### Initial setup
Settings are read only from `config.toml`.
The `config.toml` path is specified via `CONFIG_PATH`.

```sh
cp config.example.toml config.toml
vim config.toml
## Edit according to the setting list below
```

### Setting list

| Section | Key | Description | Default |
|---|---|---|---|
| `bot` | `id` | Bot identifier (for log display) | `micha-bot` |
| | `display_name` | Bot display name (for notifications and history records) | `micha-bot` |
| | `respond_all` | Whether the bot responds to all messages in the specified channels without mentions | `false` |
| `discord` | `token` * | Discord bot token | — |
| | `guild_id` * | Target server ID | — |
| | `category_ids` * | Category IDs the bot responds in | — |
| | `readonly_channel_ids` | Channels the bot patrols periodically to collect topics | empty |
| | `readonly_poll_sec` | Read-only fetch interval (seconds) | `300` |
| | `login_notice_channel_id` | Login notice destination. If empty, the first channel of the target categories | empty |
| `database` | `url` * | Postgres DB connection URL | — |
| `endpoints.<name>` | `base_url` | URL of a named endpoint. Multiple endpoints can be defined per use | — |
| | `api_key` | Endpoint API key | — |
| | `extra_headers` | Additional HTTP headers (e.g. `x-opencode-session`) | empty |
| `models` | `chat` | Response model. Specified as `"<endpoint>/<model>"` | — |
| | `embedding` | Memory model. Specified as `"<endpoint>/<model>"` | — |
| | `embedding_dim` | Long-term memory vector dimension (normally unchanged. Must match the DB) | `1536` |
| `brave` | `api_key` | Brave API key for web search. Web search is unavailable if unset | empty |
| `persona` | `path` | Persona file defining the bot personality. Uses the default persona if unset | empty |
| `random_talk` | `probability` | Probability of random bot messages in channels | `0.02` |
| `random_talk` | `cooldown_sec` | Interval between random bot messages in channels (seconds) | `1800` |
| `memory` | `history_limit` | Number of chat history entries the bot refers to | `20` |
| `memory` | `top_k` | Number of memories the bot recalls | `5` |
| `reaction` | `probability` | Probability of emoji reactions by the bot | `0.15` |
| `reaction` | `cooldown_sec` | Interval between emoji reactions by the bot (seconds) | `600` |
| `shell` | `enabled` | Whether shell command execution by the bot is allowed | `true` |
| `shell` | `image` | Docker image for shell command execution by the bot | `alpine:latest` |
| `shell` | `timeout_sec` | Shell command execution limit (seconds) | `30` |
| `server` | `health_addr` | Health check listen port | `:8080` |

Items marked with * are required.

### Starting with Docker Compose
Complete the initial setup first
```sh
git clone https://github.com/haya58/micha.git
cd micha/docker
docker compose up -d
docker compose logs -f bot  # OK if "bot started" appears
```

-  Mounts `../config.toml` into the container
- The DB (`pgvector/pgvector:pg16`) starts automatically, and migrations are applied automatically on bot startup. The DB data is stored in `botdb/` at the repository root
- Includes DinD (`dind` service) for shell execution
  - The bot connects via `DOCKER_HOST=tcp://dind:2375`.
- To run multiple bots on the same host, prepare a TOML file per persona (e.g. `config-kuro.toml`) and add a service with a different mount to `docker-compose.yml`.
  - DBs are created automatically by `create-dbs.sh`.

## Deploying to k8s
-  Edit config.toml inside `k8s/secret.yaml`
-  Apply it

```sh
kubectl apply -k .  # Run at the repository root
```
- To run multiple bots (personas), duplicate the Secret and Deployment per bot and change only the dbname in `database_url`.
- `kustomization.yaml` bundles the manifests in `k8s/` with the persona files in `persona/`.
- Rewrite the `files` in `kustomization.yaml` to match the persona being deployed.

The DinD sidecar (`dind` container) handles disposable execution, and the bot connects via `DOCKER_HOST=tcp://localhost:2375`. `/healthz` is set as the liveness/readiness probe.

## Building the Docker image

- Pushing to the `main` branch builds a Linux/amd64 image via `.github/workflows/docker-publish.yml`
- Pushed to `ghcr.io/<owner>/<repo>:latest` with a short SHA tag
