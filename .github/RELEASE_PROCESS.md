# Release Process

SQLWarden uses Release Please for stable versioning and changelogs, GoReleaser
for downloadable binaries, and a separate Docker workflow for multi-architecture
images in GitHub Container Registry (GHCR).

## Stable releases

1. Merge conventional commits into `main` (or a supported `release/**` branch).
2. Release Please creates or updates an `autorelease: pending` pull request. The
   pull request contains the next version, the complete changelog since the
   previous stable release, and the manifest update.
3. Review and merge the release pull request.
4. Release Please creates the stable `vX.Y.Z` tag and GitHub release.
5. The tag starts the GoReleaser and Docker workflows. GoReleaser attaches the
   platform archives and `checksums.txt`; Docker publishes the multi-architecture
   GHCR image and its stable SemVer aliases.

While SQLWarden is below `1.0.0`, fixes and features increment the patch
version. Breaking changes increment the minor version. Mark a breaking change
with `type(scope)!: description` or a `BREAKING CHANGE:` commit footer.

## Release candidates

Release candidates are operational prereleases. They do not change
`CHANGELOG.md` or `.release-please-manifest.json`; those files continue to
describe stable releases only.

Before creating a candidate:

1. Merge all intended changes into `main` or the applicable `release/**` branch.
2. Wait for Release Please to open or update the release pull request.
3. Confirm that the proposed stable version is correct.

To publish a candidate:

1. Open **Actions → Release Candidate → Run workflow**.
2. Enter the full candidate version without the `v` prefix, such as
   `0.10.0-rc.1`.
3. Leave `target_ref` as `main`, or enter the applicable `release/**` branch.
4. Run the workflow.

The workflow verifies the version against the open Release Please PR and creates
a `vX.Y.Z-rc.N` GitHub prerelease. The existing tag workflows then attach the
same platform archives and checksums as a stable release and publish a
multi-architecture image as `ghcr.io/sqlwarden/sqlwarden:X.Y.Z-rc.N`.

Candidate images never update stable `X.Y`, `X`, or `latest` tags. Candidate
GitHub release notes are cumulative from the previous stable tag, so every
candidate is independently useful to testers. The candidate workflow uses the
categorized notes from the pending Release Please pull request, with a
candidate-specific heading and warning, so its changelog format matches the
eventual stable release.

If testing finds a problem, merge the fix normally, wait for the Release Please
PR to update, and publish the next candidate number. Never move, delete, or
reuse a published candidate tag. A failed artifact workflow can be rerun against
the unchanged tag; a changed candidate must receive a new number.

When the latest candidate is approved, merge the Release Please PR normally.
The final `CHANGELOG.md` entry and stable release notes cover the entire range
from the previous stable release to the new stable release; RC tags do not split
that history.

## Published outputs

Every stable release and release candidate publishes:

- Linux, macOS, and Windows archives for amd64 and arm64
- `checksums.txt` for the archives
- A Linux amd64/arm64 image in GHCR

Stable tags publish full, minor, major, and `latest` Docker aliases as allowed by
the Docker metadata rules. Prerelease tags publish only the full prerelease and
commit-SHA aliases.

## Local validation

```bash
# Run tests and quality checks.
make audit

# Build a local snapshot with GoReleaser.
make build/release

# Check the version in a Docker build.
docker build \
  --build-arg VERSION=0.10.0-rc.1 \
  --build-arg COMMIT="$(git rev-parse HEAD)" \
  --build-arg DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t sqlwarden:release-test .
docker run --rm sqlwarden:release-test ./sqlwarden --version
```

## Troubleshooting

- **No pending Release Please PR:** wait for the Release Please workflow or fix
  the conventional commits that should cause a release.
- **Candidate version mismatch:** use the stable version proposed by the pending
  Release Please PR and append `-rc.N`.
- **Candidate tag already exists:** increment `N`; candidate tags are immutable.
- **GitHub prerelease has no archives:** rerun the failed **Artifact Release via
  GoReleaser** workflow for that tag.
- **Candidate image is missing:** rerun the failed **Docker Image** workflow for
  that tag.
- **Wrong stable version bump:** review commit types and breaking-change markers
  before merging the Release Please PR.
