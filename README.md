# dis-llm-bot (prototype)

共用 Discord チャットボット。1プロセス=1ペルソナ=1ギルド=1データベース（同一Postgresサーバ内のDB単位で分離）。

## 仕様対応表

- 単一サーバ + 複数カテゴリ: `DISCORD_GUILD_ID` + `DISCORD_CATEGORY_IDS`
- スレッド対応: 親チャンネルのカテゴリで判定
- 権限: View/Send/History が無いchは読み書きしない
- 記憶: `messages`（短期）+ `memories`（pgvector長期）
- DBはボットごと（ペルソナ単位）に分離。分離単位はPostgresサーバ内のデータベース名で、`DATABASE_URL` のdbnameをボットごとに変える（サーバ自体は共用可）
- tools: `brave_search` / `fetch_page` / `exec_shell`（OpenAI互換 function calling）
- 画像: 添付の存在を本文に付記、リンクは本文抽出してLLMに渡す
- ランダム発言: 5分tick + 確率 + ch毎クールダウン
- シェル: `docker run --rm --network none --cap-drop ALL` 使い捨て。docker必須

## 起動

```sh
cp .env.example .env
# .env を編集
go run ./cmd/bot
# or
docker compose up --build
```

マイグレーションは起動時に自動適用（`internal/store/schema.sql`）。ボットは自分のDBにだけ接続するため、他ペルソナのDBには触れない。

## 同一サーバで複数ボットを運用する

1. ペルソナごとにDBを用意する（サーバは共用）:
   ```sql
   CREATE DATABASE botdb_shiro;
   CREATE DATABASE botdb_kuro;
   ```
   docker composeの場合は `db` サービスの `EXTRA_DBS` に列挙すれば初回起動時に自動作成される:
   ```sh
   EXTRA_DBS="botdb_shiro botdb_kuro" docker compose up --build
   ```
2. ボットごとに `DATABASE_URL` のdbnameだけ変えて起動する（トークン・ペルソナも別々）:
   ```sh
   BOT_ID=shiro BOT_DB=botdb_shiro DISCORD_BOT_TOKEN=xxx ... docker compose up bot
   ```
   k8sではボットごとにDeployment/Secretを分け、`DATABASE_URL` のdbname部分だけ変える。

## 参考

- TomoriBot (Bredrumb/TomoriBot) は挙動（tool loop、memory種別、persona運用）のみ参考。コード移植なし。
