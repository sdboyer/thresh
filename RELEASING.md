# Releasing thresh

A release is cut by pushing a tag. For a `v*` tag, `.github/workflows/release.yml`:

1. runs the test workflow against the tagged commit;
2. checks that the commit is on `main`;
3. runs GoReleaser (`.goreleaser.yaml`), which publishes a GitHub release:
   - binaries for macOS and Linux, amd64 and arm64;
   - each archive holding the binary, `LICENSE`, `README.md`, `hooks/` and `skills/`;
   - a `checksums.txt`;
   - notes generated from the pull requests merged since the previous tag.

A pushed tag also makes `go install github.com/sdboyer/thresh@vX.Y.Z` resolve. Release
binaries and `go install` builds report the same `thresh version`: both read the module
version Go stamps into the binary.

## Choose the version

thresh is pre-1.0, so versions are `v0.MINOR.PATCH`:

- **Minor** for a change that can break an existing setup or change what it means. That
  covers the `.thresh` config (a field or kind renamed, removed or newly required), tap
  identity (existing verdicts no longer match), and the tap log format.
- **Patch** for everything else.

A tag with a pre-release suffix (`v0.2.0-rc.1`) publishes as a pre-release.

## Cut a release

1. Make sure `main` is green and holds everything that ships:

   ```sh
   git switch main && git pull
   ```

2. Tag the commit with a signed, annotated tag:

   ```sh
   git tag -s v0.1.0 -m "thresh v0.1.0"
   ```

3. Push the tag:

   ```sh
   git push origin v0.1.0
   ```

4. Watch the run (`gh run watch`, or the Actions tab). When it finishes, the release is at
   `https://github.com/sdboyer/thresh/releases/tag/v0.1.0`. Edit the generated notes there
   if they need it.

## Rehearse a release locally

```sh
goreleaser release --snapshot --clean
./dist/thresh_darwin_arm64*/thresh version
```

This builds every archive into `dist/` (gitignored) without publishing anything.

## When a release fails

Never reuse a version once its tag has been pushed. The Go module proxy and checksum
database may already have recorded that tag's commit, and a moved tag then fails to verify
for anyone who runs `go install` on it.

1. Fix the problem on `main`.
2. Tag the next patch version.
3. Delete the failed tag, so it can't be mistaken for a release:

   ```sh
   git push --delete origin vX.Y.Z && git tag -d vX.Y.Z
   ```
