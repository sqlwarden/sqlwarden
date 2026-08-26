# Desktop release process

SQLWarden desktop artifacts are built by `.github/workflows/desktop-release.yml` and attached to the same GitHub release as the server artifacts. The desktop workflow builds from the release tag, signs on native runners, and uploads nothing to the GitHub release until every platform has passed verification.

The first supported targets are:

- Windows 10/11 x64 as a per-user NSIS installer.
- macOS 11 or newer as a universal DMG for Intel and Apple Silicon.
- Linux x64 as a Debian package and portable tarball using WebKit2GTK 4.1.

The release includes SHA-256 checksums, a machine-readable manifest, and GitHub artifact attestations. Automatic update checks and installation are intentionally not part of this workflow.

## GitHub environment

Create a protected environment named `desktop-release`. Limit deployment access to trusted release branches and maintainers. Add the following environment secrets without exposing them as repository variables or workflow logs.

Windows secrets:

- `WINDOWS_SIGNING_PFX_BASE64`: Base64-encoded code-signing PFX.
- `WINDOWS_SIGNING_PFX_PASSWORD`: Password protecting the PFX.

The optional environment variable `WINDOWS_TIMESTAMP_URL` may override the default `http://timestamp.digicert.com` RFC 3161 timestamp service.

On PowerShell, encode a PFX with:

```powershell
[Convert]::ToBase64String([IO.File]::ReadAllBytes('sqlwarden-signing.pfx')) | Set-Clipboard
```

The certificate must contain a private key and the Code Signing enhanced key usage. The workflow imports it only into the temporary runner profile and removes it after the build.

Apple secrets:

- `APPLE_DEVELOPER_ID_P12_BASE64`: Base64-encoded Developer ID Application certificate and private key.
- `APPLE_DEVELOPER_ID_P12_PASSWORD`: Password protecting the P12.
- `APPLE_NOTARY_PRIVATE_KEY_BASE64`: Base64-encoded App Store Connect API `.p8` key.

Apple environment variables:

- `APPLE_SIGNING_IDENTITY`: Full `Developer ID Application: ... (TEAMID)` identity.
- `APPLE_TEAM_ID`: Apple Developer team identifier.
- `APPLE_NOTARY_KEY_ID`: App Store Connect API key identifier.
- `APPLE_NOTARY_ISSUER_ID`: App Store Connect API issuer identifier.

Create the Developer ID certificate through the Apple Developer account, export it with its private key as a P12, and create an App Store Connect API key with access to the notarization service. The workflow uses a temporary keychain and removes both the keychain and API key after notarization.

## Credential validation

For a same-repository pull request, add the `desktop-release-test` label to run the real signing pipeline against the pull request merge commit. The workflow uses a synthetic `0.0.0-pr.<number>` version, requires approval for the protected `desktop-release` environment, and cannot publish to a GitHub release. Fork pull requests are rejected before any signing job can access the environment. Remove and reapply the label to validate a newer commit.

Before the first release, run **Signed Desktop Release** manually with an existing tag and leave `publish` disabled. This exercises Authenticode signing, silent Windows installation, universal macOS signing and notarization, Linux packaging, checksums, and attestations. The completed bundle remains a workflow artifact and is not added to the public release.

A tag push publishes automatically. Manual publication is available for recovery by running the workflow with an existing tag and enabling `publish`. Uploads use replacement semantics, so rerunning the same tag does not duplicate assets.

## Verification

Verify a downloaded Windows installer:

```powershell
Get-AuthenticodeSignature .\sqlwarden-desktop_*_windows_x86_64_setup.exe | Format-List
```

The status must be `Valid` and the timestamp must be present. Authenticode establishes publisher identity, but an organization with a restrictive Windows Application Control allowlist may still need to approve the SQLWarden publisher explicitly.

Verify a macOS DMG:

```sh
xcrun stapler validate sqlwarden-desktop_*_darwin_universal.dmg
spctl --assess --type open --context context:primary-signature --verbose=2 sqlwarden-desktop_*_darwin_universal.dmg
```

Verify checksums and GitHub provenance:

```sh
sha256sum --check sqlwarden-desktop_*_checksums.txt
gh attestation verify sqlwarden-desktop_*_linux_x86_64.deb --repo sqlwarden/sqlwarden
```

Run checksum verification from the directory containing all four native artifacts referenced by the checksum file.

## Failure behavior

- A missing certificate, invalid signature, rejected notarization, missing artifact, or checksum failure stops the workflow before GitHub release upload.
- Signing failures never fall back to unsigned Windows or macOS publication.
- The existing server GoReleaser workflow remains independent.
- If Release Please has not created the matching GitHub release yet, publication waits briefly and then fails without deleting the tag or native workflow artifacts.

The repository currently has no root project license. Desktop archives must not claim or synthesize one; they include only the existing third-party Bytebase MIT notice until the repository-hardening work establishes the project license.
