# dis-llm-bot

共用 Discord チャットボット。1プロセス = 1ペルソナ = 1ギルド = 1データベース。
外部仕様は `docs/spec-external.md`、内部仕様は `docs/spec-internal.md` を参照する。

## 設定

設定は `config.toml` のみから読み込む（環境変数は `CONFIG_PATH` を除き参照しない）。

```sh
cp config.example.toml config.toml
# config.toml を編集
```

`config.toml` は秘密情報を含むため git 管理外である。手元で `go run` する場合は、`database_url` のホスト名 `db` が名前解決できるよう `/etc/hosts` に `127.0.0.1 db` を追加するか、接続先を手元のPostgresに書き換える。

## 手元での起動

```sh
go run ./cmd/bot
```

ヘルス確認：`curl localhost:8080/healthz` が `ok` を返せばDB接続も正常である。

## Docker Composeでの起動

```sh
cd deploy
docker compose up --build -d
docker compose logs -f bot  # "bot started" が出ればDiscord接続成功
```

* `../config.toml` をコンテナにマウントして使用する。DBは `pgvector/pgvector:pg16` が自動起動する。マイグレーションはBot起動時に自動適用される。
* シェル実行用にDinD（`dind` サービス）が同梱される。Botは `DOCKER_HOST=tcp://dind:2375` で接続する。
* 同一サーバで複数Botを運用する場合は、ペルソナごとにTOMLファイル（例：`config-kuro.toml`）を用意し、`docker-compose.yml` のマウント先を変えたサービスを追加する。DBは `EXTRA_DBS` で自動作成する。

## k8s環境へのデプロイ

1. `deploy/k8s.yaml` の Secret `bot-config-toml` 内のTOML（トークン・ギルドID・カテゴリID・DB接続先）を置換する。
2. Bot（ペルソナ）ごとに Secret・Deployment を複製し、`database_url` のdbname部分だけ変える。
3. 適用する：

```sh
kubectl apply -f deploy/k8s.yaml
```

DinDサイドカー（`dind` コンテナ）が使い捨て実行を担い、Botは `DOCKER_HOST=tcp://localhost:2375` で接続する。`/healthz` がliveness/readiness probeに設定済みである。

## 参考

- TomoriBot (Bredrumb/TomoriBot) は挙動（tool loop、memory種別、persona運用）のみ参考。コード移植なし。
