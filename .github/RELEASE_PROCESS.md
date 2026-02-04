# Release Automation Quick Reference

## What Was Implemented

### GitHub Actions Workflows

1. **`.github/workflows/release-please.yml`**
   - Runs on every push to `main`
   - Creates/updates release PRs with changelog
   - Creates GitHub releases when release PR is merged

2. **`.github/workflows/goreleaser.yml`**
   - Triggered when a version tag is created (e.g., `v1.0.0`)
   - Builds binaries for: Linux, macOS, Windows (amd64 & arm64)
   - Uploads artifacts to GitHub releases
   - Generates checksums and SBOMs

3. **`.github/workflows/conventional-commits.yml`**
   - Runs on all pull requests
   - Validates commit messages and PR titles
   - Blocks merge if commits don't follow conventional format

### Configuration Files

- **`.goreleaser.yml`**: GoReleaser configuration for multi-platform builds
- **`release-please-config.json`**: Release-please settings for changelog and versioning
- **`.release-please-manifest.json`**: Current version tracker (starts at 0.1.0)
- **`.commitlintrc.json`**: Commit message linting rules

### Documentation

- **`CONTRIBUTING.md`**: Complete guide to conventional commits and release process

### Code Changes

- **`internal/version/version.go`**: Updated to support version injection from goreleaser
- **`Makefile`**: Added `build` with version injection and `build/release` targets
- **`README.md`**: Updated with release process documentation

## How to Use

### Making Changes

1. **Create commits following conventional format:**
   ```bash
   git commit -m "feat: add user authentication"
   git commit -m "fix: prevent memory leak in database pool"
   git commit -m "docs: update API documentation"
   ```

2. **Create PR with conventional title:**
   ```
   feat: add OAuth2 support
   fix(api): handle null pointer in user handler
   ```

### Releasing

1. **Push to main** (or merge PR) with conventional commits
2. **Release-please creates/updates a release PR** automatically
3. **Review the release PR** - check changelog and version bump
4. **Merge the release PR** - this triggers:
   - Tag creation (e.g., `v0.2.0`)
   - GitHub release creation
   - GoReleaser builds binaries for all platforms
   - Artifacts uploaded to release

### Commit Types & Version Bumps

- `feat:` → Minor bump (0.1.0 → 0.2.0)
- `fix:` → Patch bump (0.1.0 → 0.1.1)
- `feat!:` or `BREAKING CHANGE:` → Major bump (0.1.0 → 1.0.0)
- `docs:`, `chore:`, `ci:` → No version bump (appear in changelog but don't trigger release)

### Local Development

```bash
# Build with version injection
make build

# Test release build locally (requires goreleaser)
make build/release

# Run tests
make test

# Code quality checks
make audit
```

## First Release

To create your first release:

1. **Update initial version** in `.release-please-manifest.json` if needed (currently set to 0.1.0)

2. **Make commits with conventional format:**
   ```bash
   git add .
   git commit -m "chore: setup automated releases with release-please and goreleaser"
   git push origin main
   ```

3. **Wait for release-please** to create the first release PR

4. **Merge the release PR** to publish v0.1.0

## Troubleshooting

- **PR checks failing**: Ensure commits follow conventional format
- **No release PR created**: Check that commits are on `main` branch
- **Release build fails**: Verify `.goreleaser.yml` configuration
- **Wrong version bump**: Review commit message types

## Examples

### Breaking Change
```bash
git commit -m "feat!: redesign authentication API

BREAKING CHANGE: The /auth endpoint now requires OAuth2 tokens instead of API keys."
```

### Feature with Scope
```bash
git commit -m "feat(database): add connection pooling with configurable limits"
```

### Bug Fix
```bash
git commit -m "fix(api): prevent SQL injection in user search"
```

### Multiple Changes (separate commits)
```bash
git commit -m "feat: add rate limiting middleware"
git commit -m "docs: update rate limiting configuration"
git commit -m "test: add rate limiting tests"
```
