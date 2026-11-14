# 内部仕様

dis-llm-bot の内部構成を定義する。言語は Go、依存は discordgo と pgx のみである。

## 構成

```
cmd/bot/main.go            起動・healthz・スケジューラ結線
internal/config            環境変数の読み込み
internal/discord           受信フィルタ・応答生成・進捗embed・ログイン通知・エラー通知
internal/llm               OpenAI互換クライアント（chat・embeddings・Vision・自動再試行）
internal/tools             brave_search・fetch_page・exec_shell
internal/memory            短期履歴と長期記憶の取得・プロンプト整形
internal/store             Postgres/pgvector アクセス・起動時マイグレーション
internal/persona           デフォルトペルソナ埋込・外部ファイル読込
internal/scheduler         ランダム発言・読み取り専用チャンネルの定期取得
internal/textutil          UTF-8安全な切詰・無効バイト除去
```

## 処理フロー

1. 起動：設定読込→DB接続＋マイグレーション→Discord接続→ログイン通知→スケジューラ起動。
2. 受信：ギルド→カテゴリ／スレッド解決→読み取り専用除外→権限確認→応答条件判定。
3. 応答：履歴（直近20件）＋類似記憶（Top5）＋読み取り専用話題＋ペルソナでプロンプト構築→LLM呼出（tool最大3往復・並列実行）→接頭辞除去→2000字分割送信→確率的リアクション付与。
4. 工具：`brave_search`（Brave API）・`fetch_page`（SSRF対策・ボット判定排除・DNS8秒上限）・`exec_shell`（docker CLI経由でDinDに使い捨て実行）・`remember`（記憶保存）。
5. 記憶：全発言を `messages` にupsert保存。40字超または `remember` 呼出で `memories` にembedding付き保存。
6. 定期：ランダム発言（5分tick・確率・クールダウン）、読み取り専用取得（既定300秒・Discord IDでupsert）。

## LLMクライアントの適応

- `max_tokens`・非既定 `temperature`・`reasoning_effort` 既定値の拒否（400）に対し、段階的に置換して最大4回再試行する。
- Visionは `image_url` パートで添付URLを直接渡す。
- embeddings非対応endpointでは記憶検索を省略し履歴のみで応答する。

## データ

- `messages`: 全発言。`discord_id` のUNIQUE制約でupsertし、ポーリングの冪等性を保証する。
- `memories`: 長期記憶。`embedding vector(1536)`＋HNSW索引、コサイン類似でTop5検索する。
- `tool_runs`: tool実行記録。
- マイグレーションは `internal/store/schema.sql` を起動時に冪等適用する。

## 設定

設定の唯一の供給源はTOMLファイル（`CONFIG_PATH`、既定 `./config.toml`、雛形は `config.example.toml`）である。環境変数は参照しない（`CONFIG_PATH` 自体を除く）。

## デプロイ

- `docker/Dockerfile`：Alpine＋docker-cliの単一イメージ。
- `docker/docker-compose.yml`：bot＋pgvector＋DinD。`EXTRA_DBS` でペルソナ用DBを自動作成する。DB初期化スクリプトはルートの `create-dbs.sh` をマウントする。
- `k8s/`：Secret・ConfigMap・Deploymentに分割。TOML全体をSecret搭載しBotごとに複製する。

## 制約

- `store`・`scheduler` は実DB要のため単体テスト対象外である。
- 無料モデル等の一部endpointは外部利用不可・embeddings未提供の場合がある。
