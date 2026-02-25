# Act Migration Tickets

## TICKET-ACT-001: Act Input Routing Integration (Web)
- Priority: P0
- Goal: `selectedActId` ベースで入力ルーティングを本番フローへ統合する。
- Scope:
  - `useActSelection` を Home/Interaction フローに接続。
  - 入力時に `routeInputToAct` を適用。
  - `selectedActId == null` のとき新規 `act` ノード作成。
  - `selectedActId != null` のとき既存 `act` へ入力送信。
- Main files:
  - `/workspaces/Insightify/InsightifyWeb/src/features/act/hooks/useActSelection.ts`
  - `/workspaces/Insightify/InsightifyWeb/src/features/act/model/routeInputToAct.ts`
  - `/workspaces/Insightify/InsightifyWeb/src/features/interaction/hooks/useInteractionFlow.ts`
  - `/workspaces/Insightify/InsightifyWeb/src/pages/Home.tsx`
- Acceptance:
  - 未選択入力で毎回新規 `act` が作成される。
  - 選択中入力で同一 `act` の `timeline` が更新される。

## TICKET-ACT-002: Act Node Renderer (Web)
- Priority: P0
- Goal: `llmChat` 中心表示から `act` 単一ノード表示へ移行する。
- Scope:
  - `ActNode` コンポーネント実装。
  - `status/mode/timeline/pending_actions` の表示。
  - ノード選択 UI を `selectedActId` と連動。
- Main files:
  - `/workspaces/Insightify/InsightifyWeb/src/components/graph/ActNode/ActNode.tsx`
  - `/workspaces/Insightify/InsightifyWeb/src/components/graph/ActNode/ActTimeline.tsx`
  - `/workspaces/Insightify/InsightifyWeb/src/features/ui/hooks/useUiNodeSync.ts`
- Acceptance:
  - `UI_NODE_TYPE_ACT` が描画される。
  - `timeline` が時系列表示される。
  - クリックで選択/解除が機能する。

## TICKET-ACT-003: Disable Direct User Node Creation (Web UX)
- Priority: P0
- Goal: ユーザーが直接ノードを作る導線を廃止する。
- Scope:
  - 「Add LLM Chat」導線を削除。
  - `runTestChatNode` 依存を act 起点導線に置換。
- Main files:
  - `/workspaces/Insightify/InsightifyWeb/src/pages/home/ActionPanel.tsx`
  - `/workspaces/Insightify/InsightifyWeb/src/pages/home/useBootstrap.ts`
  - `/workspaces/Insightify/InsightifyWeb/src/pages/home/useHomeChatNodeCreator.ts`
- Acceptance:
  - UI 上で直接ノード作成ができない。
  - 新規入力のみが act 作成トリガーになる。

## TICKET-ACT-004: Act Orchestrator Service (Core)
- Priority: P0
- Goal: Core 側に `act` の状態機械と分岐実行を実装する。
- Scope:
  - `planning -> suggest/search/run_worker -> needs_user_action/done/failed` の遷移管理。
  - `UiActState.timeline` へのイベント追記ロジック。
  - worker選定失敗時に `autonomous_executor` を起動。
- Main files:
  - `/workspaces/Insightify/InsightifyCore/internal/domain/act/act.go`
  - `/workspaces/Insightify/InsightifyCore/internal/gateway/service/ui`
  - `/workspaces/Insightify/InsightifyCore/internal/gateway/service/worker`
- Acceptance:
  - act 状態遷移が定義通りに動作する。
  - すべての遷移で `timeline` が追記される。
  - 選定失敗時フォールバックが発火する。

## TICKET-ACT-005: Worker Routing Policy (Core)
- Priority: P1
- Goal: worker 選定ロジックを明文化し実装する。
- Scope:
  - ルールベース分類（suggest/search/run）。
  - 信頼度閾値とフォールバック起動条件。
  - `allowed_workers` 制約の検証。
- Main files:
  - `/workspaces/Insightify/InsightifyCore/internal/workers/plan/autonomous_executor.go`
  - `/workspaces/Insightify/InsightifyCore/internal/runner/registry_plan.go`
- Acceptance:
  - 代表入力で期待 worker が選定される。
  - 低信頼ケースでフォールバック/要確認遷移が再現される。

## TICKET-ACT-006: CreateNodeInTab Actor Enforcement Hardening (Core)
- Priority: P1
- Goal: actor 制約を全経路で一貫適用する。
- Scope:
  - `CreateNodeInTab` のエラーをクライアントで明確表示。
  - `ApplyUiOps.actor` の運用ルール統一（`act|worker|system`）。
  - 監査ログに actor を出力。
- Main files:
  - `/workspaces/Insightify/InsightifyCore/internal/gateway/service/ui/service.go`
  - `/workspaces/Insightify/InsightifyWeb/src/features/ui/hooks/useUiEditor.ts`
- Acceptance:
  - 不正 actor は常に拒否される。
  - actor 情報が追跡可能。

## TICKET-ACT-007: Restore Compatibility for Act Payload (Core/Web)
- Priority: P1
- Goal: 復元時に `act` payload が欠損しないことを保証する。
- Scope:
  - restore/applyops の `act` 正規化確認。
  - `timeline`/`pending_actions` の round-trip テスト。
- Main files:
  - `/workspaces/Insightify/InsightifyCore/internal/gateway/service/ui`
  - `/workspaces/Insightify/InsightifyWeb/src/features/ui/api.ts`
  - `/workspaces/Insightify/InsightifyCore/internal/gateway/integration`
- Acceptance:
  - 復元後に `status/mode/timeline` が一致する。

## TICKET-ACT-008: Remove Remaining LLM Chat-Centric Paths
- Priority: P2
- Goal: act 完全移行後に `llmChat` 前提経路を削除する。
- Scope:
  - 使われていない `llmChat` 作成/同期処理の削除。
  - ドキュメント更新（`act_mvp_spec.md`, `act_contract.md`）。
- Main files:
  - `/workspaces/Insightify/InsightifyWeb/src/features/ui/hooks/useUiNodeSync.ts`
  - `/workspaces/Insightify/InsightifyWeb/src/features/interaction/hooks/useInteractionFlow.ts`
  - `/workspaces/Insightify/InsightifyCore/docs/act_mvp_spec.md`
- Acceptance:
  - act メインフローのみで主要E2Eが通る。

## Suggested Execution Order
1. TICKET-ACT-001
2. TICKET-ACT-002
3. TICKET-ACT-003
4. TICKET-ACT-004
5. TICKET-ACT-005
6. TICKET-ACT-006
7. TICKET-ACT-007
8. TICKET-ACT-008
