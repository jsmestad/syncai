# Release SyncAI

SyncAI releases are built for macOS and Linux, signed with GitHub's keyless OIDC identity, and accompanied by checksums, SBOMs, and GitHub artifact attestations. The workflow keeps the GitHub release in draft state until its archives have been attested.

## Prerequisites

Maintainers need push access to `jsmestad/syncai`, permission to create signed Git tags, Go 1.26, GoReleaser 2.17.1, Staticcheck 0.8.0, govulncheck 1.7.0, Syft 1.51.0, Cosign 3.1.3, and GitHub CLI authentication. Git must sign release tags with the SSH key pinned in `.github/release-allowed-signers`, fingerprint `SHA256:gzZhJSoQdqTzhdWVTZDuSFIa3XvvzsK7k/Z6JBtHaQw`.

Before enabling releases, protect `main` with required pull-request review and blocked force pushes, protect `v*` tags from updates and deletion, restrict their creation to the release maintainer, and enable immutable GitHub releases. The workflow also rejects lightweight tags, untrusted tag signatures, and commits that are not reachable from `origin/main`.

## Publish a version

Set the next version, then validate the exact clean commit that will receive the tag:

```bash
VERSION=v1.0.0
git status --short
make ci
goreleaser release --snapshot --clean
git status --short
```

The status output must be empty before and after validation. Inspect `dist/` and confirm it contains four `.tar.gz` archives, one for each macOS or Linux and AMD64 or ARM64 pair, plus `checksums.txt` and archive SBOMs. Snapshot validation does not sign or publish artifacts.

Create one signed, annotated tag on that validated commit and push only the tag:

```bash
git tag -s "$VERSION" -m "syncai $VERSION"
git push origin "$VERSION"
```

The `Release` workflow reruns `make ci`, builds the four archives, uses the matching `CHANGELOG.md` entry as the release notes, signs `checksums.txt` through Cosign and GitHub OIDC, creates a draft GitHub release, attests every archive, and publishes the release only after attestation succeeds. A missing changelog entry stops the release.

Watch the workflow and inspect the published assets:

```bash
gh run list --repo jsmestad/syncai --workflow release.yml --limit 5
gh release view "$VERSION" --repo jsmestad/syncai
```

## Verify a release

Download the release into a new directory:

```bash
VERSION=v1.0.0
RELEASE_DIR="$(mktemp -d)"
gh release download "$VERSION" --repo jsmestad/syncai --dir "$RELEASE_DIR"
```

Verify every file covered by the SHA-256 checksum manifest:

```bash
(cd "$RELEASE_DIR" && shasum -a 256 -c checksums.txt)
```

Verify that GitHub's OIDC identity for the tagged release workflow signed the checksum manifest:

```bash
cosign verify-blob --bundle "$RELEASE_DIR/checksums.txt.bundle" --certificate-identity "https://github.com/jsmestad/syncai/.github/workflows/release.yml@refs/tags/$VERSION" --certificate-oidc-issuer "https://token.actions.githubusercontent.com" "$RELEASE_DIR/checksums.txt"
```

Verify GitHub's artifact attestation for every release archive:

```bash
for archive in "$RELEASE_DIR"/*.tar.gz; do gh attestation verify "$archive" --repo jsmestad/syncai --signer-workflow jsmestad/syncai/.github/workflows/release.yml --source-ref "refs/tags/$VERSION"; done
```

Inspect an archive before installing it. Each archive must contain only the `syncai` binary, `LICENSE`, and `README.md`:

```bash
tar -tzf "$RELEASE_DIR/syncai_1.0.0_darwin_arm64.tar.gz"
```

## Remove a bad release

Delete a release that contains a broken or unsafe artifact, but leave its Git tag in place so the version cannot be reused:

```bash
VERSION=v1.0.0
gh release delete "$VERSION" --repo jsmestad/syncai --yes
```

Fix the problem on a new commit, increment the patch version, and run the complete release process again. Never recreate, move, or repush the deleted version's tag.
