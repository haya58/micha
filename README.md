# dis-llm-bot (prototype)

共用 Discord チャットボット。1プロセス=1ペルソナ=1ギルド=1物理DB。

## 仕様対応表

- 単一サーバ + 複数カテゴリ: `DISCORD_GUILD_ID` + `DISCORD_CATEGORY_IDS`
- スレッド対応: 親チャンネルのカテゴリで判定
- 権限: View/Send/History が無いchは読み書きしない
- 記憶: `messages`（短期）+ `memories`（pgvector長期）
- DBはボットごと（ペルソナ単位）に物理分離。`DATABASE_URL` で切替
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

マイグレーションは起動時に自動適用（`internal/store/schema.sql`）。

## 参考

- TomoriBot (Bredrumb/TomoriBot) は挙動（tool loop、memory種別、persona運用）のみ参考。コード移植なし。
