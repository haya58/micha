# Micha
## 概要
Micha  (ミカ と発音) は Discord コミュニティをより盛り上げるための LLM チャット Bot アプリケーションです。
シンプルで堅牢ながら、現代の LLM チャット Bot に必要な機能を備えています。

## 設定

設定は `config.toml` のみから読み込む（環境変数は `CONFIG_PATH` を除き参照しない）。

```sh
cp config.example.toml config.toml
# config.toml を編集
```

`config.toml` は秘密情報を含むため git 管理外である。手元で `go run` する場合は、`database_url` のホスト名 `db` が名前解決できるよう `/etc/hosts` に `127.0.0.1 db` を追加するか、接続先を手元のPostgresに書き換える。

### 設定項目一覧

| セクション | キー | 内容 | 既定値 |
|---|---|---|---|
| `bot` | `id` | Bot識別名（ログ表示用） | `default-bot` |
| | `display_name` | Botの表示名（通知・履歴記録用） | `dis-llm-bot` |
| | `respond_all` | メンション無し全発言に応答する | `false` |
| `discord` | `token` ※ | Botトークン | — |
| | `guild_id` ※ | 対象サーバID | — |
| | `category_ids` ※ | 応答対象カテゴリID | — |
| | `readonly_channel_ids` | 読取専用チャンネルID | 空 |
| | `readonly_poll_sec` | 読取専用の取込間隔（秒） | `300` |
| | `login_notice_channel_id` | ログイン通知先。空時は対象カテゴリ先頭 | 空 |
| `database` | `url` ※ | 専用DBへの接続先 | — |
| `openai` | `base_url` | OpenAI互換endpoint | `https://api.openai.com/v1` |
| | `api_key` ※ | APIキー | — |
| | `model` | 応答モデル | `gpt-4o-mini` |
| | `embedding_model` | 記憶用embeddingモデル | `text-embedding-3-small` |
| | `embedding_dim` | ベクトル次元（DBと一致させる） | `1536` |
| | `extra_headers` | 追加HTTPヘッダ（例：`x-opencode-session`） | 空 |
| `brave` | `api_key` | Brave検索キー（未設定時は検索不可） | 空 |
| `persona` | `path` | ペルソナファイル。空時は内蔵既定 | 空 |
| `random_talk` | `probability`／`cooldown_sec` | ランダム発言の確率・間隔 | `0.02`／`1800` |
| `memory` | `history_limit`／`top_k` | 参照履歴件数・想起記憶件数 | `20`／`5` |
| `reaction` | `probability`／`cooldown_sec` | リアクション確率・間隔 | `0.15`／`600` |
| `shell` | `enabled`／`image`／`timeout_sec` | 使い捨て実行の可否・イメージ・上限 | 有効／`alpine:latest`／`30` |
| `server` | `health_addr` | ヘルスチェック待受 | `:8080` |

※は必須項目である。

## 手元での起動

```sh
go run ./cmd/bot
```

ヘルス確認：`curl localhost:8080/healthz` が `ok` を返せばDB接続も正常である。

## Docker Composeでの起動

```sh
cd docker
docker compose up --build -d
docker compose logs -f bot  # "bot started" が出ればDiscord接続成功
```

* `../config.toml` をコンテナにマウントして使用する。DBは `pgvector/pgvector:pg16` が自動起動する。マイグレーションはBot起動時に自動適用される。
* シェル実行用にDinD（`dind` サービス）が同梱される。Botは `DOCKER_HOST=tcp://dind:2375` で接続する。
* 同一サーバで複数Botを運用する場合は、ペルソナごとにTOMLファイル（例：`config-kuro.toml`）を用意し、`docker-compose.yml` のマウント先を変えたサービスを追加する。DBは `EXTRA_DBS` で自動作成する。

## k8s環境へのデプロイ

1. `k8s/secret.yaml` のTOML（トークン・ギルドID・カテゴリID・DB接続先）を置換する。
2. Bot（ペルソナ）ごとに Secret・Deployment を複製し、`database_url` のdbname部分だけ変える。
3. 適用する：

```sh
kubectl apply -k .
```

`kustomization.yaml` が `k8s/` 内マニフェストと `persona/` 内ペルソナファイルを束ねる。デプロイ対象のペルソナに合わせて `kustomization.yaml` の `files` を書き換える。

DinDサイドカー（`dind` コンテナ）が使い捨て実行を担い、Botは `DOCKER_HOST=tcp://localhost:2375` で接続する。`/healthz` がliveness/readiness probeに設定済みである。

## 継続的デリバリ

`main` ブランチへのpushで `.github/workflows/docker-publish.yml` がLinux/AMD64イメージをビルドし、`ghcr.io/<owner>/<repo>:latest` と short SHAタグでプッシュする。

## 参考

- TomoriBot (Bredrumb/TomoriBot) は挙動（tool loop、memory種別、persona運用）のみ参考。コード移植なし。
