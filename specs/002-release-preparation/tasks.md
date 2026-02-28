# Tasks: v0.1.0 リリース準備

**Input**: Design documents from `specs/002-release-preparation/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/

**Tests**: テスト専用タスクは不要（既存テストの通過確認のみ）

**Organization**: タスクはユーザーストーリー単位で整理。各ストーリーは独立して実装・検証可能。

## Format: `[ID] [P?] [Story] Description`

- **[P]**: 並列実行可能（異なるファイル、依存関係なし）
- **[Story]**: 対応するユーザーストーリー（US1, US2, US3, US4, US5）
- 各タスクに正確なファイルパスを含む

---

## Phase 1: Setup（事前準備）

**Purpose**: ローカル環境の確認とツールインストール

- [x] T001 GoReleaser のインストール確認（`goreleaser --version`、未インストールの場合は `brew install goreleaser`）
- [x] T002 [P] golangci-lint のインストール確認（`golangci-lint --version`、未インストールの場合は `brew install golangci-lint`）
- [x] T003 [P] ローカルでユニットテスト通過を確認（`go test ./internal/... -race -count=1`）
- [x] T004 [P] ローカルでビルド通過を確認（`go build ./cmd/worktree-container/`）

---

## Phase 2: Foundational（基盤修正）

**Purpose**: CI/CD の Go バージョン不整合を解消。この修正がすべての後続タスクの前提。

**⚠️ CRITICAL**: go.mod（`go 1.25.0`）と CI（Go 1.22/1.23）の不整合を解消しないと CI が通らない

- [x] T005 `.github/workflows/ci.yml` の Go バージョンマトリクスを `["1.22", "1.23"]` → `["1.25"]` に修正
- [x] T006 [P] `.github/workflows/ci.yml` の lint ジョブの Go バージョンを `"1.23"` → `"1.25"` に修正
- [x] T007 [P] `.github/workflows/ci.yml` の cross-compile ジョブの Go バージョンを `"1.23"` → `"1.25"` に修正
- [x] T008 [P] `.github/workflows/release.yml` の Go バージョンを `"1.23"` → `"1.25"` に修正

**Checkpoint**: CI 設定ファイルが go.mod と整合している状態

---

## Phase 3: User Story 1 — CI/CD パイプラインの動作検証 (Priority: P1) 🎯 MVP

**Goal**: GitHub Actions の CI が全マトリクスで通過し、コードがリリース可能な品質であることを確認する

**Independent Test**: ブランチを push し、GitHub Actions の全ジョブ（build, lint, cross-compile）が成功すること

### Implementation for User Story 1

- [x] T009 [US1] ローカルで golangci-lint を実行し lint エラーがないことを確認（`golangci-lint run`）
- [ ] T010 [US1] ブランチを push して GitHub Actions CI ワークフローの実行を開始
- [ ] T011 [US1] GitHub Actions の build ジョブが全マトリクスで成功することを確認
- [ ] T012 [US1] GitHub Actions の lint ジョブが成功することを確認
- [ ] T013 [US1] GitHub Actions の cross-compile ジョブが成功することを確認（darwin/linux/windows × amd64/arm64）

**Checkpoint**: CI が全ジョブ通過。リリース品質のコードであることが保証された状態

---

## Phase 4: User Story 2 — GoReleaser スナップショットビルド検証 (Priority: P1)

**Goal**: GoReleaser のスナップショットビルドで全プラットフォーム向けアーティファクトが正しく生成されることを確認する

**Independent Test**: `goreleaser release --snapshot --clean` が成功し、`dist/` に 5 ターゲットのアーティファクトが存在すること

### Implementation for User Story 2

- [x] T014 [US2] `goreleaser release --snapshot --clean` を実行してスナップショットビルドを実行
- [x] T015 [US2] `dist/` ディレクトリに darwin_amd64, darwin_arm64, linux_amd64, linux_arm64, windows_amd64 の各アーティファクトが存在することを確認
- [x] T016 [US2] `dist/` ディレクトリに `checksums.txt` が存在することを確認
- [x] T017 [US2] ローカルプラットフォーム用バイナリで `--version` を実行し、バージョン情報が出力されることを確認

**Checkpoint**: GoReleaser がリリース時に正しくアーティファクトを生成できることが保証された状態

---

## Phase 5: User Story 3 — Homebrew Tap リポジトリの準備 (Priority: P2)

**Goal**: Homebrew Tap リポジトリが GitHub 上に存在し、GoReleaser が Formula を自動 push できる状態にする

**Independent Test**: `shinji-kodama/homebrew-tap` リポジトリが存在し、`Formula/` ディレクトリがあり、`HOMEBREW_TAP_TOKEN` が設定されていること

### Implementation for User Story 3

- [ ] T018 [US3] `shinji-kodama/homebrew-tap` リポジトリの存在を確認（`gh repo view shinji-kodama/homebrew-tap`）、存在しない場合は作成（`gh repo create shinji-kodama/homebrew-tap --public`）（**リリース時にユーザーが実施** — docs/RELEASE.md 参照）
- [ ] T019 [US3] Homebrew Tap リポジトリに `Formula/` ディレクトリと README.md を作成してコミット・push（**リリース時にユーザーが実施** — docs/RELEASE.md 参照）
- [ ] T020 [US3] GitHub Personal Access Token（PAT）を作成し、`shinji-kodama/homebrew-tap` への書き込み権限を付与（**ユーザー手動操作** — docs/RELEASE.md 参照）
- [ ] T021 [US3] `worktree-container` リポジトリの GitHub Secrets に `HOMEBREW_TAP_TOKEN` を設定（`gh secret set HOMEBREW_TAP_TOKEN`）（**ユーザー手動操作** — docs/RELEASE.md 参照）
- [x] T022 [US3] `.goreleaser.yml` の `brews` セクションの設定が正しいことを確認（`repository.owner`, `repository.name`, `directory: Formula`）

**Checkpoint**: Homebrew Tap の準備が完了。リリースタグ push 時に GoReleaser が自動的に Formula を push できる状態

---

## Phase 6: User Story 4 — WinGet マニフェストの準備 (Priority: P2)

**Goal**: WinGet マニフェストテンプレートを準備し、リリース後に `microsoft/winget-pkgs` に PR を提出できる状態にする

**Independent Test**: `packaging/winget/` に 3 つのマニフェストテンプレートファイルが存在し、WinGet スキーマ v1.9.0 に準拠していること

### Implementation for User Story 4

- [x] T023 [US4] `packaging/winget/` ディレクトリを作成
- [x] T024 [P] [US4] WinGet version マニフェストテンプレートを作成: `packaging/winget/shinji-kodama.worktree-container.yaml`（ManifestType: version、プレースホルダー: `{{VERSION}}`）
- [x] T025 [P] [US4] WinGet installer マニフェストテンプレートを作成: `packaging/winget/shinji-kodama.worktree-container.installer.yaml`（InstallerType: zip、NestedInstallerType: portable、x64/arm64 対応、プレースホルダー: `{{VERSION}}`, `{{SHA256_X64}}`, `{{SHA256_ARM64}}`）
- [x] T026 [P] [US4] WinGet defaultLocale マニフェストテンプレートを作成: `packaging/winget/shinji-kodama.worktree-container.locale.en-US.yaml`（Publisher, PackageName, License, ShortDescription, Tags を含む）
- [x] T027 [US4] 3 つのマニフェストファイルが WinGet スキーマ v1.9.0 の必須フィールドをすべて含むことをレビュー

**Checkpoint**: WinGet マニフェストテンプレートが準備完了。リリース後にプレースホルダーを置換して PR 提出可能な状態

---

## Phase 7: User Story 5 — リリース手順書の整備 (Priority: P3)

**Goal**: 初回リリースおよび今後のリリースで使えるチェックリスト形式のリリース手順書を作成する

**Independent Test**: チェックリストの各項目が具体的で実行可能であり、前提条件・手順・検証方法が明記されていること

### Implementation for User Story 5

- [x] T028 [US5] `docs/RELEASE.md` にリリース手順チェックリストを作成（前提条件セクション、初回リリース手順、通常リリース手順、WinGet 提出手順を含む）
- [x] T029 [US5] `docs/RELEASE.md` の初回リリース固有セクションに Homebrew Tap 作成、PAT 設定、GitHub Secrets 登録の手順を含める
- [x] T030 [US5] `docs/RELEASE.md` の通常リリースセクションにタグ作成→push→CI 確認→WinGet PR 提出の手順を含める

**Checkpoint**: リリース手順書が完成。初回リリースのドライランを実行可能な状態

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: 全ストーリーにまたがる最終確認

- [ ] T031 全修正をコミットし、PR を作成（Conventional Commits 形式: `chore: v0.1.0 リリース準備`）
- [ ] T032 PR の CI が全ジョブ通過することを最終確認
- [ ] T033 PR をマージ後、main ブランチでの CI 通過を確認

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: 依存なし — 即座に開始可能
- **Foundational (Phase 2)**: Setup 完了後 — **全ユーザーストーリーをブロック**
- **US1 (Phase 3)**: Foundational 完了後 — CI 通過の確認
- **US2 (Phase 4)**: Foundational 完了後 — スナップショットビルド（US1 と並列可能だがローカル実行）
- **US3 (Phase 5)**: Foundational 完了後 — Homebrew Tap 準備（US1/US2 と独立）
- **US4 (Phase 6)**: 依存なし — WinGet テンプレート作成（他ストーリーと完全独立）
- **US5 (Phase 7)**: US3, US4 完了後推奨 — 手順書にすべての準備内容を反映
- **Polish (Phase 8)**: 全ストーリー完了後

### User Story Dependencies

- **US1 (CI 検証)**: Phase 2 完了が必須。他ストーリーへの依存なし
- **US2 (GoReleaser)**: Phase 2 完了が必須。US1 と並列実行可能
- **US3 (Homebrew Tap)**: Phase 2 完了が必須。US1/US2 と並列実行可能
- **US4 (WinGet)**: 完全に独立。Phase 1 と並列でも実行可能
- **US5 (手順書)**: US3/US4 の内容を手順書に含めるため、これらの完了を推奨

### Parallel Opportunities

- T001〜T004: Setup タスクは並列実行可能
- T005〜T008: CI 修正は並列実行可能（異なるファイル箇所だが同一ファイル含む）
- T024〜T026: WinGet マニフェスト 3 ファイルは並列作成可能
- US1/US2/US3: Phase 2 完了後に並列実行可能
- US4: 完全独立、いつでも実行可能

---

## Parallel Example: User Story 4 (WinGet)

```bash
# WinGet マニフェスト 3 ファイルを並列作成:
Task: "Create version manifest in packaging/winget/shinji-kodama.worktree-container.yaml"
Task: "Create installer manifest in packaging/winget/shinji-kodama.worktree-container.installer.yaml"
Task: "Create defaultLocale manifest in packaging/winget/shinji-kodama.worktree-container.locale.en-US.yaml"
```

---

## Implementation Strategy

### MVP First (User Story 1 + 2)

1. Phase 1: Setup（ツール確認）
2. Phase 2: Foundational（CI の Go バージョン修正）
3. Phase 3: User Story 1（CI 通過確認）
4. Phase 4: User Story 2（GoReleaser スナップショット）
5. **STOP and VALIDATE**: リリースパイプラインの基本動作が保証された状態

### Incremental Delivery

1. Setup + Foundational → CI が動作する基盤
2. US1（CI 検証） → リリース品質の保証
3. US2（GoReleaser） → アーティファクト生成の保証
4. US3（Homebrew） + US4（WinGet） → パッケージマネージャ配布準備
5. US5（手順書） → リリースプロセスの文書化
6. Polish → PR 作成・マージ

---

## Notes

- T020, T021 は**ユーザー手動操作**が必要（PAT 作成と Secrets 設定）
- US4（WinGet）は他のストーリーと完全に独立しており、いつでも実行可能
- `dist/` ディレクトリは `.gitignore` に含まれるべき（GoReleaser の一時出力）
- コミットは Conventional Commits 形式（`chore:`, `fix:`, `ci:` 等）を使用
