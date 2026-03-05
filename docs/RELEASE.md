# Release Procedure Checklist

This document defines the release process for `loam`.

## Prerequisites

- [ ] Go >= 1.25 is installed
- [ ] GoReleaser is installed (`brew install goreleaser`)
- [ ] golangci-lint is installed (`brew install golangci-lint`)
- [ ] GitHub CLI (`gh`) is installed and authenticated
- [ ] Docker Desktop is running
- [ ] The `main` branch is up to date

---

## First Release Only Steps

These steps are only required for the initial release (v0.1.0). They can be skipped for subsequent releases.

### Creating the Homebrew Tap Repository

- [ ] Create the `mmr-tortoise/homebrew-tap` repository

  ```bash
  gh repo create mmr-tortoise/homebrew-tap --public \
    --description "Homebrew tap for mmr-tortoise packages"
  ```

- [ ] Create the `Formula/` directory and README.md, then commit and push

  ```bash
  gh repo clone mmr-tortoise/homebrew-tap
  cd homebrew-tap
  mkdir Formula
  echo "# Homebrew Tap" > README.md
  git add .
  git commit -m "chore: initialize"
  git push
  ```

### Creating a GitHub Personal Access Token (PAT)

- [ ] Create a new PAT under GitHub Settings > Developer settings > Personal access tokens > Fine-grained tokens
  - Token name: `HOMEBREW_TAP_TOKEN`
  - Repository access: `mmr-tortoise/homebrew-tap` only
  - Permissions: Contents (Read and write)

### Configuring GitHub Secrets

- [ ] Set `HOMEBREW_TAP_TOKEN` in the `loam` repository Secrets

  ```bash
  gh secret set HOMEBREW_TAP_TOKEN --repo mmr-tortoise/loam
  ```

---

## Standard Release Procedure

Steps to follow for every release.

### 1. Pre-release Checks

- [ ] Confirm you are working on the `main` branch

  ```bash
  git checkout main
  git pull origin main
  ```

- [ ] Confirm all unit tests pass

  ```bash
  go test ./internal/... -race -count=1
  ```

- [ ] Confirm there are no lint errors

  ```bash
  golangci-lint run
  ```

- [ ] Confirm the build succeeds

  ```bash
  go build ./cmd/loam/
  ```

### 2. Verify GoReleaser Snapshot Build

- [ ] Run a snapshot build

  ```bash
  goreleaser release --snapshot --clean
  ```

- [ ] Confirm the following artifacts exist in the `dist/` directory
  - `loam_*_darwin_amd64.tar.gz`
  - `loam_*_darwin_arm64.tar.gz`
  - `loam_*_linux_amd64.tar.gz`
  - `loam_*_linux_arm64.tar.gz`
  - `loam_*_windows_amd64.zip`
  - `loam_*_windows_arm64.zip`
  - `checksums.txt`

- [ ] Verify `--version` with the binary for your local platform

  ```bash
  ./dist/loam_*/loam --version
  ```

### 3. Create and Push the Version Tag

- [ ] Create and push the version tag

  ```bash
  git tag v<VERSION>
  git push origin v<VERSION>
  ```

### 4. Verify the GitHub Actions Release Workflow

- [ ] Confirm the Release workflow runs automatically

  ```bash
  gh run watch
  ```

- [ ] Confirm artifacts are correctly uploaded to the GitHub Release page

  ```bash
  gh release view v<VERSION>
  ```

### 5. Verify the Homebrew Formula

- [ ] Confirm the Formula has been pushed to the `mmr-tortoise/homebrew-tap` repository

  ```bash
  gh api repos/mmr-tortoise/homebrew-tap/contents/Formula/loam.rb
  ```

- [ ] Confirm it can be installed via Homebrew (optional)

  ```bash
  brew install mmr-tortoise/tap/loam
  loam --version
  ```

---

## WinGet Manifest Submission Procedure

This is done manually after a release.

### 1. Obtain the SHA256 Hash

- [ ] Get the SHA256 hash for the Windows binaries from `checksums.txt` in the GitHub Release

  ```bash
  gh release download v<VERSION> --pattern checksums.txt
  grep windows checksums.txt
  ```

### 2. Update the Manifest Templates

- [ ] Replace the placeholders in the templates with actual values

  ```bash
  cd packaging/winget/
  # Replace VERSION, SHA256_X64, SHA256_ARM64, RELEASE_DATE
  sed -i '' 's/{{VERSION}}/<VERSION>/g' *.yaml
  sed -i '' 's/{{SHA256_X64}}/<SHA256_X64>/g' *.yaml
  sed -i '' 's/{{SHA256_ARM64}}/<SHA256_ARM64>/g' *.yaml
  sed -i '' 's/{{RELEASE_DATE}}/<RELEASE_DATE>/g' *.yaml
  ```

### 3. Submit the PR

- [ ] Fork the `microsoft/winget-pkgs` repository
- [ ] Copy the manifest files to your forked repository

  ```bash
  # Place them under manifests/s/mmr-tortoise/loam/<VERSION>/
  mkdir -p manifests/s/mmr-tortoise/loam/<VERSION>/
  cp packaging/winget/*.yaml manifests/s/mmr-tortoise/loam/<VERSION>/
  ```

- [ ] Submit a PR and wait for the WinGet team's review

---

## Troubleshooting

### GoReleaser Fails

```bash
# Validate the configuration
goreleaser check

# Run a snapshot build with verbose logging
goreleaser release --snapshot --clean --verbose
```

### Homebrew Formula Is Not Pushed

- Check the permissions of `HOMEBREW_TAP_TOKEN`
- Check the expiration date of the PAT
- Confirm the `mmr-tortoise/homebrew-tap` repository exists

### CI Fails with a Go Version Error

- Confirm that the `go` directive in `go.mod` matches the `go-version` in `.github/workflows/ci.yml`
