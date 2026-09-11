# Contributing to SQLWarden

Thank you for contributing to SQLWarden! This document provides guidelines for contributing to the project.

## Conventional Commits

This project uses [Conventional Commits](https://www.conventionalcommits.org/) for commit messages. This enables automatic changelog generation and semantic versioning.

### Commit Message Format

Each commit message should follow this format:

```
<type>(<scope>): <subject>

<body>

<footer>
```

#### Type

Must be one of the following:

- **feat**: A new feature
- **fix**: A bug fix
- **docs**: Documentation only changes
- **style**: Changes that do not affect the meaning of the code (white-space, formatting, etc)
- **refactor**: A code change that neither fixes a bug nor adds a feature
- **perf**: A code change that improves performance
- **test**: Adding missing tests or correcting existing tests
- **build**: Changes that affect the build system or external dependencies
- **ci**: Changes to CI configuration files and scripts
- **chore**: Other changes that don't modify src or test files
- **revert**: Reverts a previous commit

#### Scope (Optional)

The scope should be the name of the package affected (e.g., `database`, `api`, `auth`, etc.)

#### Subject

The subject contains a succinct description of the change:

- Use the imperative, present tense: "change" not "changed" nor "changes"
- Don't capitalize the first letter
- No period (.) at the end

#### Examples

```
feat(auth): add JWT token validation

This implements JWT token validation middleware to ensure
all authenticated requests have valid tokens.

Closes #123
```

```
fix(database): prevent SQL injection in user queries

Updates the user query builder to use parameterized queries
instead of string concatenation.
```

```
docs: update installation instructions in README
```

```
perf(api): improve response time for /users endpoint
```

### Breaking Changes

Breaking changes should be indicated by an exclamation mark `!` before the colon:

```
feat(api)!: remove deprecated /v1/users endpoint

BREAKING CHANGE: The /v1/users endpoint has been removed.
Use /v2/users instead.
```

### Pull Request Title

Pull request titles must also follow the conventional commits format. For squash-merged PRs the title becomes the resulting commit message on `main`, so keep it conventional. For rebase-merged PRs, each individual commit must follow conventional commits as well — see [Merge Strategy](#merge-strategy).

## Development Workflow

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/amazing-feature`)
3. Make your changes
4. Write tests for your changes
5. Ensure all tests pass (`make test`)
6. Commit your changes using conventional commits
7. Push to your fork
8. Open a Pull Request with a conventional commit title

## Merge Strategy

`main` is kept linear. A PR must be rebased onto the latest `main` before it merges. If another PR merges first, rebase onto the new `main` rather than merging `main` into your branch.

Whether a PR is squash merged or rebase merged depends on what its commit history represents:

- **Squash merge** — the PR is a single feature or fix, even if the branch has many small, incremental commits made for review convenience. Those commits are collapsed into one commit on `main`, using the PR title as the message.
- **Rebase merge** — the PR is a list of independent changes, where each commit is a complete feature or fix in its own right and should remain visible as its own commit in `main`'s history. Every commit in the branch must independently follow conventional commits.

A PR must not mix both patterns: a list of independently meaningful commits alongside a separate run of small commits that only make sense squashed together as one feature. Split that PR in two instead:

1. One PR with only the independently meaningful commits, rebase merged.
2. One PR with the feature's full incremental history, squash merged.

## Release Process

Releases are automated using release-please:

1. Commit messages following conventional commits determine version bumps:
   - `feat:` commits trigger a minor version bump
   - `fix:` commits trigger a patch version bump
   - `BREAKING CHANGE:` or `!` triggers a major version bump

2. When commits are pushed to `main`, release-please creates/updates a release PR

3. When the release PR is merged:
   - A new tag is created
   - A GitHub release is created with changelog
   - GoReleaser builds and uploads binaries for multiple platforms

## Questions?

If you have questions about contributing, please open an issue for discussion.
