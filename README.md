# c8y-software-uploader

A [go-c8y-cli](https://goc8ycli.netlify.app) extension which uploads software packages to the Cumulocity software repository.

It searches a directory for software packages, parses the software name, version, architecture, and type from each filename, and uploads every file as a software version. It is based on the [software-uploader example](https://github.com/reubenmiller/go-c8y/tree/v2/examples/software-uploader) of the go-c8y v2 SDK.

## Install

Requires go-c8y-cli >= [2.57.0](https://github.com/reubenmiller/go-c8y-cli/releases/tag/v2.57.0).

```sh
c8y extensions install reubenmiller/c8y-software-uploader
```

c8y downloads the binary for your platform from the latest GitHub release and verifies its checksum. Update it with:

```sh
c8y extensions update software-uploader
```

## Usage

The command uses the active go-c8y-cli session.

```sh
# Show what would be uploaded, without changing anything
c8y software-uploader upload --dir ./dist --pattern "*.deb" --dry

# Upload Debian, RPM, and Alpine packages
c8y software-uploader upload --dir ./dist --pattern "*.deb" --pattern "*.rpm" --pattern "*.apk"

# Upload a single file
c8y software-uploader upload --file ./tedge_1.4.2_arm64.deb

# Upload to another tenant
c8y software-uploader upload --dir ./dist --session other-tenant.json
```

The command writes one result per version, so the go-c8y-cli output flags work:

```sh
c8y software-uploader upload --dir ./dist --pattern "*.deb" --output table
c8y software-uploader upload --dir ./dist --pattern "*.deb" --select name,version,status --output csv
```

```
| name               | version | softwareType | architecture | status   |
|:-------------------|:--------|:-------------|:-------------|:---------|
| c8y-command-plugin | 0.3.0   | apt          | all          | created  |
| tedge              | 1.4.2   | apt          | amd64        | existing |
| tedge              | 1.4.2   | apt          | arm64        | created  |
```

| Status | Description |
|---|---|
| `created` | The version was uploaded |
| `existing` | The version already existed, and was not changed |
| `replaced` | The binary of the existing version was replaced (`--replace`) |
| `failed` | The upload failed. See the `error` property |
| `planned` | The version would be uploaded (`--dry`) |

Messages and the progress bar are written to stderr. The exit code is non-zero if any version failed.

### Flags

| Flag | Default | Description |
|---|---|---|
| `--dir` | | Directory to search (recursively) for software packages |
| `--file` | | Single software package to upload (alternative to `--dir`) |
| `--pattern` | `*` | Glob pattern of the files to upload (repeatable) |
| `--type` | | Software type of all packages. Overrides the detected type |
| `--type-map` | | Map a filename glob to a software type, for example `*.bin=firmware` (repeatable, first match wins) |
| `--version` | | Version of all packages. Overrides the detected version |
| `--name-prefix` | | Prefix added to every software name |
| `--concurrency` | `5` | Number of concurrent uploads (1–20) |
| `--replace` | | Replace the binary of versions which already exist |
| `--no-progress` | | Don't show the progress bar. It is not shown when stderr is not a terminal |
| `--log-level` | `error` | `error`, `warn`, `info`, or `debug`. `debug` also logs the HTTP requests |

The go-c8y-cli global flags `--dry`, `--session`, `--sessionMode`, `--output`, `--select`, and `--filter` work as for other c8y commands. Compared with the SDK example, `--force` is called `--replace`, and `--verbose`/`--debug` are replaced by `--log-level`, as c8y uses these flag names itself.

### Session mode

Uploads create data, so they are not allowed in the `prod` session mode. `--replace` deletes the existing binaries, so it needs the `dev` or `ci` mode. Use `--sessionMode` to change the mode for one command:

```sh
c8y software-uploader upload --dir ./dist --sessionMode dev --replace
```

## Filename parsing

| Format | Extension(s) | `softwareType` | Naming convention |
|---|---|---|---|
| Debian | `.deb` | `apt` | `name_version_arch.deb` |
| RPM | `.rpm` | `rpm` | `name-version-release.arch.rpm` |
| Alpine | `.apk` | `apk` | `name_version-release_arch.apk` |
| OpenWrt/Yocto | `.ipk` | `ipk` | `name_version_arch.ipk` |
| Arch Linux | `.pkg.tar.zst`, `.pkg.tar.xz` | `pacman` | `name-version-pkgrel-arch.pkg.tar.zst` |
| Archives | `.tar.gz`, `.tgz`, `.tar.bz2`, `.tar.xz`, `.tar.zst`, `.zip`, `.7z`, `.rar` | `archive` | `name-version_arch.tar.gz` |
| Binary | `.bin` | `binary` | `name-version.bin` |
| Java | `.jar`, `.war`, `.ear` | `java` | |
| Snap | `.snap` | `snap` | |
| Flatpak | `.flatpak` | `flatpak` | |
| AppImage | `.appimage` | `appimage` | |
| Windows | `.exe`, `.msi` | `windows` | |
| macOS | `.dmg`, `.pkg` | `macos` | |

A software item is created for each name, architecture, and type, so `tedge` for `arm64` and `amd64` are two software items.

The software type is chosen in this order:

1. `--type`, which is used for all packages
2. `--type-map`, the first matching pattern
3. The type detected from the file extension

## Finding the uploaded software

The software items have a `c8y_SoftwareUploader` fragment:

```sh
c8y inventory find --query "has(c8y_SoftwareUploader) and type eq 'c8y_Software'"
```

## Development

Build the binary into the extension folder, then install the folder as a local extension. c8y links to the folder, so you only need to rebuild the binary to see your changes.

```sh
go build -o bin/c8y-software-uploader .
c8y extensions install .
c8y extensions validate .
go test ./...
```

Releases are published by GoReleaser when a `v*` tag is pushed.
