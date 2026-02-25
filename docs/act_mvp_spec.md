# Act-Centric Interaction MVP Specification

## 1. Goal

本仕様は、Insightify の対話/ノード操作を `act` 中心に再設計するための MVP 要件を定義する。

狙い:

- ユーザー入力を「意図単位 (`act`)」へ束ねる
- ノード生成の責務を `act` と `worker` に限定する
- `act` が提案・検索・worker実行のオーケストレーションを担う

## 2. Core Concepts

### 2.1 Act

- `act` は「ユーザーが達成したいこと」を表すノード。
- `act` は単一ノードで提案・検索・worker実行の全責務を持つ。
- 提案結果・検索結果・実行結果は子ノード分割せず、`act` 内のタイムラインイベントとして保持する。
- `act` は、要求達成に必要な worker を選定し `StartRun` を起動できる。

### 2.2 Worker

- `worker` は実処理の実行主体。
- `act` から指示された目的に対して実行される。
- 実行結果は `act` の `timeline` イベントとして可視化される。

### 2.3 User

- ユーザーはノードを直接作成できない。
- ユーザーができることは入力とアクション選択のみ。
- ノード作成は `act` または `worker` が実施する。

## 3. Interaction Rules

### 3.1 Input Routing

- `selectedActId` が存在する場合:
  - 入力は選択中 `act` へ送る。
- `selectedActId` が存在しない場合:
  - 新規 `act` を作成し、その `act` へ入力を送る。

### 3.2 Node Creation Permission

- Frontend の「任意ノード作成 UI」は廃止する。
- `CreateNodeInTab` をユーザー主導で直接呼ぶ経路を禁止する。
- サーバー側は `actor` を検査し、`user` actor のノード生成を拒否する。
- 許可 actor は MVP では `act` / `worker` / `system` のみ。

### 3.3 Act Responsibilities

`act` は入力を受け取ると次のいずれかを実施する:

- 提案 (`suggest`)
  - `act` タイムラインに提案イベントを追加する
  - ユーザーに次アクション選択を促す
- 検索 (`search`)
  - `act` タイムラインに検索結果イベントを追加する
  - ユーザー要求に近い候補を提示する
- 実行 (`run_worker`)
  - 適切な worker を選定して `StartRun`
  - 実行中/完了状態を `act` の状態とタイムラインへ反映する

## 4. Act State Machine

MVP では以下の状態遷移を固定する:

- `idle`
- `planning`
- `suggesting`
- `searching`
- `running_worker`
- `needs_user_action`
- `done`
- `failed`

基本遷移:

- `idle -> planning`
- `planning -> suggesting | searching | running_worker`
- `suggesting -> needs_user_action`
- `searching -> needs_user_action`
- `running_worker -> done | needs_user_action | failed`
- `needs_user_action -> planning` (ユーザー選択後)

## 5. Data Model (MVP)

### 5.1 UiNodeType extension

`UiNodeType` に以下を追加する:

- `UI_NODE_TYPE_ACT`

`act` は単一型のみを使用し、提案・検索・実行の差分は `act.status` / `act.mode` / `timeline` で表現する。
既存 `LLM_CHAT` は移行期間の互換目的で残すが、新規主要フローでは `ACT` のみを使用する。

### 5.2 UiNode payload

`UiNode` に `act` state を保持できる payload を追加する:

- `act_id`
- `status`
- `mode` (`planning|suggest|search|run_worker|needs_user_action|done|failed`)
- `goal` (ユーザー要求の要約)
- `selected_worker` (選定された worker key)
- `pending_actions` (ユーザーに選択させる候補)
- `timeline` (提案/検索/実行ログを時系列で持つイベント配列)

提案・検索・実行結果の保持先は `timeline` とし、`act` 以外の用途別ノード型は作らない。

### 5.3 Selection state (Frontend)

Frontend はグローバルに `selectedActId: string | null` を保持する。

- ノードクリックで `selectedActId` を更新
- ペインクリックで `selectedActId = null`
- 入力送信時に 3.1 のルールを適用

## 6. API / Contract Changes

### 6.1 CreateNodeInTab policy

- `CreateNodeInTabRequest.actor` を必須として扱う（MVP運用ルール）。
- サーバーで actor ごとの許可チェックを実装。
- `user` actor での node create は `permission denied`。

### 6.2 Act operation endpoint (recommended)

既存の interaction + ApplyOps だけで実装可能だが、MVP 安定性のため以下を推奨:

- 新 RPC: `ActService.SubmitInput`
  - 入力テキスト
  - `selected_act_id` (nullable)
  - 返却: `act_id`, `status`, `mode`, 更新ドキュメント(または差分)

これにより Frontend は act routing と権限境界を単純化できる。

### 6.3 Fallback worker (new)

通常 worker 選定に失敗した場合のフォールバック worker を追加する:

- worker key: `autonomous_executor` (名称は実装時に確定)
- 役割:
  - 目的分解
  - 実行計画生成
  - 許可された worker 群を使った段階実行
  - 失敗時の再計画
- `act` は選定失敗時に本 worker を起動し、進捗を `timeline` に記録する。

## 7. Worker Selection Policy

MVP はルールベース選定で開始する:

- 入力分類:
  - `suggest` 優先キーワード
  - `search` 優先キーワード
  - 実行系キーワード
- ルールに一致した worker key を選定
- 信頼度が閾値未満なら、まずフォールバック worker (`autonomous_executor`) を検討する
- フォールバック適用不可/失敗時は `needs_user_action` へ遷移し候補 worker を提示

将来は LLM ベース選定に置換可能だが、MVP では deterministic ルールを優先する。

フォールバック worker の安全制約:

- `max_steps`
- `max_runtime_ms`
- `allowed_workers`
- `require_user_approval_on_risky_action`

## 8. Frontend Changes

必須変更:

- 「Create Chat Node」導線を削除
- 選択 act 表示 (どの act に入力されるかを UI 上で明示)
- 未選択入力時の新規 act 生成トリガー
- `act` 単一ノード内でのタイムライン表示

互換変更:

- 既存 `llmChat` 表示コンポーネントは `act` ノード内部ビューとして再利用可

## 9. Backend Changes

必須変更:

- `UiNodeType` / `UiNode` スキーマ拡張
- `CreateNodeInTab` の actor policy enforcement
- act orchestrator サービス追加（既存 worker service 連携）
- フォールバック worker (`autonomous_executor`) 追加
- restore/applyops で act ノードの `status/mode/timeline` を正しく保存・復元

## 10. Migration Plan

Phase 1 (互換導入):

- スキーマ追加
- actor policy は warning ログのみ
- frontend に selectedActId と act 入力ルーティング追加

Phase 2 (MVP切替):

- Create Chat Node 導線削除
- user actor node create を拒否
- 新規入力は常に act 起点

Phase 3 (整理):

- 旧フロー依存コード削除
- 監視/テレメトリ追加

## 11. Acceptance Criteria

- ノード未選択で入力すると新規 `act` が 1 つ作成される
- `act` 選択中入力は同一 `act` に紐づく
- ユーザーから直接ノード作成 API を叩くと拒否される
- `act` が提案/検索/実行を単一ノード内で処理し、`timeline` に記録する
- 通常 worker 選定不可時にフォールバック worker が起動する
- フォールバック worker 失敗時は `needs_user_action` に遷移する
- タブ復元後も `act` の `status/mode/timeline` が保持される

## 12. Out of Scope (MVP)

- 複数 act への同時ブロードキャスト入力
- 高度な自律プランニング（長期計画最適化）
- 完全自動 worker 合成
- ACL の詳細ロール設計（MVP は actor 文字列ベース）

## 13. Risks

- 既存 `LLM_CHAT` フローとの並行運用で状態不整合が起きる可能性
- actor policy 導入時に既存クライアントが失敗する可能性
- `timeline` イベント増加により単一ノード表示が肥大化する可能性

対策:

- 段階移行 (Phase 1-3)
- 互換ログと fail-fast エラーメッセージ
- タイムラインの折りたたみ/仮想化表示の導入
