# Micha
## 概要
Micha  (ミカ と発音) は Discord コミュニティをより盛り上げるための LLM チャット Bot アプリケーションです。
シンプルで堅牢ながら、現代の LLM チャット Bot に必要な機能を備えています。

## 特徴
- Discord サーバでユーザとチャットを通じて交流するチャットBot
- OpenAI 純正 API と OpenAI 互換の API に対応
  - OpenCode Go の API で動作確認済
- Bot が会話したり、読み取ったりできるカテゴリを制限可能な安全性に配慮した設計
- Bot へ設定する人格や指示を Markdown 形式で指定可能
- pgvector で構築したデータベースと連携した高度な長期記憶機能
- インタラクション機能
  - Bot からの自発的な発言
  - 絵文字によるランダムなインタラクション
- Bot は使い捨てコンテナでの簡易的なシェルコマンド実行可能
- Kubernetes へのデプロイにネイティブに対応

## Getting started
### 初期設定
設定は `config.toml` のみから読み込む。
config.toml のパスは `CONFIG_PATH` にて指定する。

```sh
cp config.example.toml config.toml
vim config.toml
## 以下の設定項目一覧にしたがって編集
```

### 設定項目一覧

| セクション | キー | 内容 | 既定値 |
|---|---|---|---|
| `bot` | `id` | Bot識別名（ログ表示用） | `micha-bot` |
| | `display_name` | Botの表示名（通知・履歴記録用） | `micha-bot` |
| | `respond_all` | Bot がメンション無しで指定したチャンネルの全発言に応答するか | `false` |
| `discord` | `token` * | Discord の Botトークン | — |
| | `guild_id` * | 導入対象サーバID | — |
| | `category_ids` * | Bot が応答するカテゴリID | — |
| | `readonly_channel_ids` | ここで指定したチャンネルを Bot は定期巡回し、話題を収集する | 空 |
| | `readonly_poll_sec` | 読取専用の取込間隔（秒） | `300` |
| | `login_notice_channel_id` | ログイン時の通知先。空の場合、は対象カテゴリの先頭のチャンネル | 空 |
| `database` | `url` * | Postgres DBへの接続URL | — |
| `openai` | `base_url` | OpenAI互換のエンドポイントt | `https://api.openai.com/v1` |
| | `api_key` * | OpenAI の API キー | — |
| | `model` | チャット応答に利用するモデル | `gpt-4o-mini` |
| | `embedding_model` | 長期記憶用 embedding モデル | `text-embedding-3-small` |
| | `embedding_dim` | 長期記憶用ベクトル次元（通常変更不要。DBと一致させる） | `1536` |
| | `extra_headers` | OpenAI 純正以外のAPI で利用する、追加HTTPヘッダ（例：`x-opencode-session`） | 空 |
| `brave` | `api_key` | Web 検索に使用する Brave API のキー。未設定の場合 Web 検索不可 | 空 |
| `persona` | `path` | Bot の人格を定義するペルソナファイル。未設定の場合、デフォルトのペルソナを使用 | 空 |
| `random_talk` | `probability` | Bot がチャンネルでランダム発言する確率 | `0.02` |
| `random_talk` | `cooldown_sec` | Bot がチャンネルでランダム発言する間隔(秒) | `1800` |
| `memory` | `history_limit` | Bot が参照するチャットの履歴件数 | `20` |
| `memory` | `top_k` | Bot が想起する記憶の件数 | `5` |
| `reaction` | `probability` | Bot が絵文字でリアクションする確率 | `0.15` |
| `reaction` | `cooldown_sec` | Bot が絵文字でリアクションする間隔(秒) | `600` |
| `shell` | `enabled` | Bot によるシェルコマンド実行可否 | `true` |
| `shell` | `image` | Bot によるシェルコマンド実行時の Docker イメージタグ | `alpine:latest` |
| `shell` | `timeout_sec` | Bot によるシェルコマンド実行上限(秒) | `30` |
| `server` | `health_addr` | ヘルスチェック待受ポート | `:8080` |

*は必須項目である。

### Docker Compose での起動
```sh
git clone https://github.com/haya58/micha.git
cd docker
docker compose up -d
docker compose logs -f bot  # "bot started" が出ればOK
```

-  `../config.toml` をコンテナにマウントする
- DBは `pgvector/pgvector:pg16` が自動起動し、マイグレーションはBot起動時に自動適用
- シェル実行用にDinD（`dind` サービス）が同梱される
  - Botは `DOCKER_HOST=tcp://dind:2375` で接続する。
- 同一ホストで複数Botを運用する場合は、ペルソナごとにTOMLファイル（例：`config-kuro.toml`）を用意し、`docker-compose.yml` のマウント先を変えたサービスを追加すること。
<!-- これなんだ？そんな手順あったっけ -->
  - DBは `EXTRA_DBS` で自動作成する。

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
