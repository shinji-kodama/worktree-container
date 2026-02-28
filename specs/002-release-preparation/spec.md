# Feature Specification: v0.1.0 リリース準備

**Feature Branch**: `002-release-preparation`
**Created**: 2026-02-28
**Status**: Draft
**Input**: User description: "リリースするまでに必要な工程をすべて洗い出して、それらを実行する前のブランチを作成し、そのブランチ内で動作テスト等を終わらせてから、Homebrew や WinGet へのパッケージのアップロードの準備を完全に終わらせる。"

## Clarifications

### Session 2026-02-28

- Q: WinGet マニフェストテンプレートの配置場所は？ → A: `packaging/winget/` ディレクトリに配置する

## User Scenarios & Testing *(mandatory)*

### User Story 1 - CI/CD パイプラインの動作検証 (Priority: P1)

開発者として、GitHub Actions の CI（ビルド・テスト・lint・クロスコンパイル）が全プラットフォームで通過することを確認し、コードがリリース可能な品質であることを保証したい。

**Why this priority**: リリースの前提条件。CI が通らない状態ではリリースできない。すべての後続ストーリーの基盤となる。

**Independent Test**: `002-release-preparation` ブランチを push し、GitHub Actions の CI ワークフローが全マトリクス（ubuntu-latest × Go 1.22/1.23、macos-latest × Go 1.22/1.23）で成功することを確認する。

**Acceptance Scenarios**:

1. **Given** ブランチが push されている, **When** CI ワークフローが実行される, **Then** build, lint, cross-compile の全ジョブが成功する
2. **Given** Go 1.22 と 1.23 のマトリクス, **When** テストが実行される, **Then** 全テストが race detector 付きで通過する
3. **Given** クロスコンパイルジョブ, **When** darwin/linux/windows × amd64/arm64 でビルドされる, **Then** すべてのターゲットでビルドが成功する

---

### User Story 2 - GoReleaser によるスナップショットビルド検証 (Priority: P1)

開発者として、GoReleaser のスナップショットビルド（`goreleaser release --snapshot --clean`）がローカルで正常に動作し、リリースアーティファクト（tar.gz/zip/checksums）が正しく生成されることを確認したい。

**Why this priority**: タグを打つ前にリリースパイプラインの動作を検証する最終関門。スナップショットビルドが失敗する場合、実際のリリースも失敗する。

**Independent Test**: ローカルで `goreleaser release --snapshot --clean` を実行し、`dist/` ディレクトリに全プラットフォーム向けのアーティファクトが生成されることを確認する。

**Acceptance Scenarios**:

1. **Given** GoReleaser がインストールされている, **When** `goreleaser release --snapshot --clean` を実行する, **Then** エラーなく完了する
2. **Given** スナップショットビルドが完了した, **When** `dist/` ディレクトリを確認する, **Then** darwin_amd64/darwin_arm64/linux_amd64/linux_arm64/windows_amd64 の各アーティファクトと checksums.txt が存在する
3. **Given** 生成されたバイナリ, **When** ローカルプラットフォーム用バイナリで `--version` を実行する, **Then** バージョン情報が正しく表示される

---

### User Story 3 - Homebrew Tap リポジトリの準備 (Priority: P2)

開発者として、Homebrew Tap 用のリポジトリ（`shinji-kodama/homebrew-tap`）が GitHub 上に存在し、GoReleaser が Formula を自動的に push できる状態にしたい。

**Why this priority**: macOS/Linux ユーザーにとっての主要なインストール手段。リリースタグを打った際に GoReleaser が自動的に Formula を生成・push するため、事前にリポジトリと認証トークンを準備する必要がある。

**Independent Test**: Homebrew Tap リポジトリの存在と `HOMEBREW_TAP_TOKEN` シークレットの設定を確認し、GoReleaser のスナップショットビルドで Formula テンプレートが生成されることを確認する。

**Acceptance Scenarios**:

1. **Given** GitHub アカウント, **When** `shinji-kodama/homebrew-tap` リポジトリを確認する, **Then** リポジトリが存在し、`Formula/` ディレクトリが作成されている
2. **Given** Homebrew Tap リポジトリが存在する, **When** `worktree-container` リポジトリの GitHub Secrets を確認する, **Then** `HOMEBREW_TAP_TOKEN` が設定されている
3. **Given** スナップショットビルドが完了した, **When** GoReleaser の出力を確認する, **Then** Homebrew Formula のテンプレートが正しく生成されている

---

### User Story 4 - WinGet マニフェストの準備 (Priority: P2)

開発者として、WinGet パッケージマニフェストのテンプレートを準備し、リリース後に `microsoft/winget-pkgs` リポジトリに PR を提出できる状態にしたい。

**Why this priority**: Windows ユーザーにとっての主要なインストール手段。WinGet は Homebrew と異なり GoReleaser による自動化が標準でないため、マニフェストテンプレートの手動準備が必要。

**Independent Test**: WinGet マニフェストテンプレートファイル群が WinGet マニフェストスキーマに準拠した正しいフォーマットで作成されていることを確認する。

**Acceptance Scenarios**:

1. **Given** WinGet マニフェストテンプレートが作成されている, **When** リリースバージョンとダウンロード URL のプレースホルダを確認する, **Then** `installer.yaml`, `defaultLocale.yaml`, `version.yaml` が WinGet スキーマに準拠している
2. **Given** マニフェストテンプレートが存在する, **When** リリース後にバージョンと URL を埋める, **Then** `microsoft/winget-pkgs` に PR を提出できるフォーマットになっている

---

### User Story 5 - リリース手順書の整備 (Priority: P3)

開発者として、初回リリースおよび今後のリリースで使えるリリース手順チェックリストを作成し、手順の見落としを防止したい。

**Why this priority**: リリースプロセスの再現性を保証する。初回リリースは特に手順が多いため、チェックリストがあることで漏れを防止できる。

**Independent Test**: チェックリストの各項目を順番に確認し、すべてのステップが具体的で実行可能であることを確認する。

**Acceptance Scenarios**:

1. **Given** リリースチェックリストが作成されている, **When** 各項目を順番に確認する, **Then** 前提条件・実行手順・検証方法がすべて明確に記載されている
2. **Given** GoReleaser の changelog 設定, **When** Conventional Commits のログを確認する, **Then** 自動生成される Release Notes が適切な内容を含む

---

### Edge Cases

- go.mod の Go バージョン（1.25.0）と CI マトリクスの Go バージョン（1.22/1.23）に不整合がある場合、ビルドやテストに影響があるか？
- `HOMEBREW_TAP_TOKEN` が未設定の状態でリリースワークフローが実行された場合、Homebrew 以外のステップ（GitHub Release の作成）は正常に完了するか？
- Windows 向けアーカイブの拡張子が zip であること、およびバイナリに `.exe` 拡張子が付与されていることを GoReleaser が正しく処理するか？
- GoReleaser が CI 環境（ubuntu-latest）で macOS/Windows 向けバイナリをクロスコンパイルする際、CGO_ENABLED=0 で問題ないか？

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: すべての既存ユニットテストが race detector 付きで通過しなければならない（MUST）
- **FR-002**: GoReleaser のスナップショットビルドが全ターゲットプラットフォーム（darwin/linux/windows × amd64/arm64）で成功しなければならない（MUST）
- **FR-003**: 生成されたバイナリが `--version` フラグでバージョン情報を正しく表示しなければならない（MUST）
- **FR-004**: GitHub Actions の CI ワークフローが全マトリクスで通過しなければならない（MUST）
- **FR-005**: Homebrew Tap リポジトリ（`shinji-kodama/homebrew-tap`）が GitHub 上に存在し、`Formula/` ディレクトリが用意されていなければならない（MUST）
- **FR-006**: `HOMEBREW_TAP_TOKEN` シークレットがリリース用リポジトリの GitHub Secrets に設定されていなければならない（MUST）
- **FR-007**: GoReleaser の設定ファイルが Homebrew Formula を正しく生成する設定を含まなければならない（MUST）
- **FR-008**: WinGet マニフェストテンプレート（`installer.yaml`, `defaultLocale.yaml`, `version.yaml`）が `packaging/winget/` ディレクトリに準備されていなければならない（MUST）
- **FR-009**: リリース実行手順がチェックリストとして文書化されていなければならない（MUST）
- **FR-010**: go.mod の Go バージョンと CI マトリクスの Go バージョンに整合性がなければならない（MUST）
- **FR-011**: GitHub Actions の release ワークフローが `v*` タグの push をトリガーとして GoReleaser を実行しなければならない（MUST）

### Key Entities

- **リリースアーティファクト**: GoReleaser が生成するバイナリアーカイブ（tar.gz/zip）とチェックサムファイル
- **Homebrew Formula**: Homebrew Tap リポジトリ内の Ruby ファイル。バイナリのダウンロード URL、SHA256、インストール手順を定義
- **WinGet マニフェスト**: `microsoft/winget-pkgs` リポジトリに提出する YAML ファイル群。パッケージの識別子、バージョン、インストーラ URL を定義
- **GitHub Secrets**: リリースワークフローで使用する認証トークン（`GITHUB_TOKEN`, `HOMEBREW_TAP_TOKEN`）

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: GitHub Actions CI が全マトリクス（4 パターン）で 100% 成功する
- **SC-002**: GoReleaser スナップショットビルドが 5 ターゲット（darwin_amd64, darwin_arm64, linux_amd64, linux_arm64, windows_amd64）すべてでアーティファクトを生成する
- **SC-003**: ローカルプラットフォーム用バイナリで `--version` が意図したバージョン文字列を出力する
- **SC-004**: macOS/Linux ユーザーが `brew install shinji-kodama/tap/worktree-container` でインストールできるための Homebrew Tap が準備されている
- **SC-005**: Windows ユーザーが `winget install` でインストールできるための WinGet マニフェストテンプレートが準備されている
- **SC-006**: リリースチェックリストに従って初回リリースのドライランを実行でき、すべてのステップで手戻りが発生しない

## Assumptions

- 初回リリースのバージョンは `v0.1.0` とする（MVP として全コマンドが実装済み）
- Homebrew Tap リポジトリ `shinji-kodama/homebrew-tap` は本フィーチャーの中で作成する（まだ存在しない可能性がある）
- WinGet マニフェストはリリース後に `microsoft/winget-pkgs` リポジトリに PR を提出する手動フローとする（GoReleaser による自動化は行わない）
- `HOMEBREW_TAP_TOKEN` は GitHub Personal Access Token（classic または fine-grained）で、Homebrew Tap リポジトリへの書き込み権限が必要
- GoReleaser はローカルにインストール済みであるか、本フィーチャーの中でインストールする
- golangci-lint はローカルにインストール済みであるか、本フィーチャーの中でインストールする
