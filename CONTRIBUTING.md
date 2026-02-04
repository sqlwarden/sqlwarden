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

Pull request titles must also follow the conventional commits format. The PR title is used when squash merging, so it's important to keep it conventional.

## Development Workflow

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/amazing-feature`)
3. Make your changes
4. Write tests for your changes
5. Ensure all tests pass (`make test`)
6. Commit your changes using conventional commits
7. Push to your fork
8. Open a Pull Request with a conventional commit title

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
