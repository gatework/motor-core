# Releasing Motor Core

The module is `github.com/gatework/motor-core`, with v0/v1 tags only. VERSION
and the matching CHANGELOG section define the next release. Historical upstream
v2 tags are not part of this repository. Release operations require authorization.

1. Run `make check`, review API migration notes, then push the reviewed branch
   and merge its PR after CI passes. Keep the shared standards copies unchanged.
2. CI runs the complete gate, then archives that exact clean Git tree as
   `motor-core-<VERSION>.tar.gz`, with REVISION and SHA256SUMS. The immutable
   artifact is named `source-<commit>` and must come from successful main CI.
3. Tag that verified main commit with VERSION and push the tag. Release automation
   checks tag/version equality and requires successful same-commit main CI. It
   downloads the CI artifact by ID, checks its digest, manifest, and every source
   file against the tag tree. It never rebuilds release content locally.
4. The workflow creates a draft, uploads the three assets, downloads and compares
   their bytes with CI, then publishes. A rerun verifies an already public release;
   it only replaces draft assets, not published assets. Missing/expired artifacts
   require successful CI again, not an unverified local substitute.
5. Read back the public tag, asset bytes, release status and module resolution.
   Install consumers using the published version with GOWORK=off and no replace.

The manual Release workflow accepts an existing tag to resume or verify a failed
publication. Never move a published tag. VERSION changes and new code require a
new CI run and a new version. The library source archive includes the Apache
license and attribution; there are no prebuilt platform binaries in this release.

The initial release automation reuses the same artifact-verification approach
as other gatework modules; source archives are CI products, not GitHub's generated
"Source code" download links.

CI runs Redis integration through `REDIS_URL`. MySQL integration is opt-in via
`MYSQL_DSN` and optionally `MYSQL_REPLICA_DSN`; an unavailable configured fixture
fails. Artifacts are retained for 30 days and cannot be overwritten. Both package
and release jobs have explicit time limits. Once an artifact expires, do not
substitute a local build or move a published tag; prepare a new checked version.
