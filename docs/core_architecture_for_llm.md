# Insightify Core Architecture (for LLM)

このドキュメントは、`InsightifyCore` の実装構成を LLM が読める形で要約したものです。

## 1. Core 全体像

- エントリポイントは `InsightifyCore/cmd/gateway/main.go`。
- 依存注入と構成生成は `InsightifyCore/internal/gateway/app/app.go`。
- Gateway は HTTP/2(h2c) で `Connect RPC` ハンドラを公開し、`interaction` だけ別途 WebSocket を公開。

主要ソース:
- `InsightifyCore/cmd/gateway/main.go`
- `InsightifyCore/internal/gateway/app/app.go`
- `InsightifyCore/internal/gateway/server/server.go`
- `InsightifyCore/internal/gateway/server/routes.go`

## 2. Gateway 構成

`NewMux` で次を登録しています。

- Connect RPC:
  - `ProjectService`
  - `RunService`
  - `UiService`
- Debug/補助エンドポイント:
  - `/ws/interaction` (WebSocket)
  - `/debug/frontend-trace`
  - `/debug/run-logs`

主要ソース:
- `InsightifyCore/internal/gateway/server/routes.go`
- `InsightifyCore/internal/gateway/handler/rpc/project.go`
- `InsightifyCore/internal/gateway/handler/rpc/run.go`
- `InsightifyCore/internal/gateway/handler/rpc/ui.go`

対応する RPC スキーマ:
- `schema/proto/insightify/v1/project.proto`
- `schema/proto/insightify/v1/run.proto`
- `schema/proto/insightify/v1/ui.proto`

## 3. Worker は Stateless + Runtime 注入

### 3.1 実行モデル

- Run 開始時、`worker.Service` は `ProjectReader.EnsureRunContext(projectID)` で `RunEnvironment` を取得。
- 実行は `runner.ExecuteWorker(ctx, runtime, workerID, params)` に委譲。
- Worker は `WorkerSpec.Run(ctx, input, runtime Runtime)` で `runtime` インターフェイスを受け取り、必要依存をそこから取得。

主要ソース:
- `InsightifyCore/internal/gateway/service/worker/run.go`
- `InsightifyCore/internal/gateway/service/project/adapter.go`
- `InsightifyCore/internal/gateway/service/project/service.go`
- `InsightifyCore/internal/runner/executor.go`
- `InsightifyCore/internal/runner/runtime.go`

### 3.2 runtime に何が入るか

`RunRuntime` は `runner.Runtime` を実装し、以下を提供します。

- Repo/Artifact FS
- Worker resolver (`SpecResolver`)
- MCP registry
- LLM client
- fingerprint salt / deps policy

主要ソース:
- `InsightifyCore/internal/gateway/service/worker/runtime.go`
- `InsightifyCore/internal/runner/runtime.go`

### 3.3 Stateless 性の意味

- 各 worker 実装は service 内状態に直接依存せず、`input + runtime interface` で実行される。
- run 固有状態（runID, interaction waiter など）は `context` と `runtime` で受け渡す。
- 例: `testllmChatNode` は context から `runID`/`InteractionWaiter` を解決して対話待ちを行う。

主要ソース:
- `InsightifyCore/internal/runner/interaction_context.go`
- `InsightifyCore/internal/runner/registry_testworker.go`

## 4. フロントとの通信

## 4.1 基本は Connect RPC

Frontend は `@connectrpc/connect-web` で transport を作り、`Project/Run/Ui` クライアントを呼びます。

主要ソース:
- `InsightifyWeb/src/rpc/transport.ts`
- `InsightifyWeb/src/rpc/clients.ts`
- `InsightifyWeb/src/features/worker/api.ts`

## 4.2 interaction だけ WebSocket + RPC schema

- スキーマ定義は `UserInteractionService` (`Wait/Send/Close`)。
- ただし現在の gateway ルーティングでは `UserInteractionService` の Connect ハンドラは登録せず、`/ws/interaction` を使用。
- WebSocket ペイロードは `wait_state / send_ack / close_ack / assistant_message` などを JSON でやり取りし、意味論は `user_interaction.proto` の Request/Response と整合。

主要ソース:
- `schema/proto/insightify/v1/user_interaction.proto`
- `InsightifyCore/internal/gateway/server/routes.go`
- `InsightifyCore/internal/gateway/handler/rpc/user_interaction.go`
- `InsightifyCore/internal/gateway/service/user_interaction/service.go`
- `InsightifyWeb/src/features/interaction/api.ts`

## 5. 追加で書いておくと良い項目

- worker ID 一覧と責務 (`registry_*.go` から自動抽出推奨)
- run lifecycle（StartRun -> execute -> UI/Artifact 反映）
- エラーコード規約（`connect.Code*` へのマッピング方針）
- interaction WS の状態遷移図（`waiting/closed/inputQueue/outputQueue`）
- 永続化境界（project/ui/artifact の store と in-memory state の切り分け）

主要ソース:
- `InsightifyCore/internal/runner/registry_architecture.go`
- `InsightifyCore/internal/runner/registry_codebase.go`
- `InsightifyCore/internal/runner/registry_plan.go`
- `InsightifyCore/internal/runner/registry_infra.go`
- `InsightifyCore/internal/gateway/service/worker/run.go`
- `InsightifyCore/internal/gateway/service/user_interaction/service.go`
