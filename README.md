# srpmproc
Upstream package importer with auto patching. Reference implementation for OpenPatch

## Usage
```
Usage:
  srpmproc [flags]
  srpmproc [command]

Available Commands:
  fetch       
  help        Help about any command

Flags:
      --basic-password string           Basic auth password
      --basic-username string           Basic auth username
      --branch-prefix string            Branch prefix (replaces import-branch-prefix) (default "r")
      --branch-suffix string            Branch suffix to use for imported branches
      --cdn string                      CDN URL shortcuts for well-known distros, auto-assigns --cdn-url.  Valid values:  rocky8, rocky, fedora, centos, centos-stream.  Setting this overrides --cdn-url
      --cdn-url string                  CDN URL to download blobs from. Simple URL follows default rocky/centos patterns. Can be customized using macros (see docs) (default "https://git.centos.org/sources")
      --git-committer-email string      Email of committer (default "rockyautomation@rockylinux.org")
      --git-committer-name string       Name of committer (default "rockyautomation")
  -h, --help                            help for srpmproc
      --import-branch-prefix string     Import branch prefix (default "c")
      --manual-commits string           Comma separated branch and commit list for packages with broken release tags (Format: BRANCH:HASH)
      --module-fallback-stream string   Override fallback stream. Some module packages are published as collections and mostly use the same stream name, some of them deviate from the main stream
      --module-mode                     If enabled, imports a module instead of a package
      --module-prefix string            Where to retrieve modules if exists. Only used when source-rpm is a git repo (default "https://git.centos.org/modules")
      --no-dup-mode                     If enabled, skips already imported tags
      --no-storage-download             If enabled, blobs are always downloaded from upstream
      --no-storage-upload               If enabled, blobs are not uploaded to blob storage
      --package-release string          Package release to fetch
      --package-version string          Package version to fetch
      --rpm-prefix string               Where to retrieve SRPM content. Only used when source-rpm is not a local file (default "https://git.centos.org/rpms")
      --single-tag string               If set, only this tag is imported
      --source-rpm string               Location of RPM to process
      --ssh-key-location string         Location of the SSH key to use to authenticate against upstream
      --ssh-user string                 SSH User (default "git")
      --storage-addr string             Bucket to use as blob storage
      --strict-branch-mode              If enabled, only branches with the calculated name are imported and not prefix only
      --taglessmode                     Tagless mode:  If set, pull the latest commit from the branch and determine version numbers from spec file.  This is auto-tried if tags aren't found.
      --tmpfs-mode string               If set, packages are imported to path and patched but not pushed
      --upstream-prefix string          Upstream git repository prefix
      --version int                     Upstream version

Use "srpmproc [command] --help" for more information about a command.
```

<br />

## Examples:

1. Import the kernel package from git.centos.org/rpms/, to local folder /opt/gitroot/rpms/kernel.git/ .  Download the lookaside source tarballs from the default CentOS file server location to local folder `/opt/fake_s3/` .  We want to grab branch "c8" (import prefix plus RHEL version), and it will be committed as branch "r8" (branch prefix plus RHEL version).  This assumes that `/opt/fake_s3` exists, and `/opt/gitroot/rpms/kernel.git` exists and is a git repository of some kind (even an empty one).

```
srpmproc --branch-prefix "r"  --import-branch-prefix "c"  --rpm-prefix "https://git.centos.org/rpms" --version 8 --storage-addr file:///opt/fake_s3  --upstream-prefix file:///opt/gitroot   --cdn centos --strict-branch-mode --source-rpm kernel
```

<br />

## CDN and --cdn-url
The --cdn-url option allows for Go-style templates to craft complex URL patterns.  These templates are: `{{.Name}}` (package name), `{{.Hash}}` (hash of lookaside file), `{{.Hashtype}}` (hash type of file, like "sha256" or "sha512"), `{{.Branch}}` (the branch we are importing), and `{{.Filename}}` (the lookaside file's name as it appears in SOURCES/).  You can add these values as part of --cdn-url to craft your lookaside pattern.


For example, if I wanted my lookaside downloads to come from CentOS 9 Stream, I would use as part of my command:
```
--cdn-url "https://sources.stream.centos.org/sources/rpms/{{.Name}}/{{.Filename}}/{{.Hashtype}}/{{.Hash}}/{{.Filename}}"
```


**Default Behavior:**  If these templates are not used, the default behavior of `--cdn-url` is to fall back on the traditional RHEL import pattern:  `<CDN_URL>/<NAME>/<BRANCH>/<HASH>` .  If that fails, a further fallback is attempted, the simple: `<CDN_URL>/<HASH>`.  These cover the common Rocky Linux and RHEL/CentOS imports if the base lookaside URL is the only thing given.  If no `--cdn-url` is specified, it defaults to "https://git.centos.org/sources" (for RHEL imports into Rocky Linux)


**CDN Shorthand:** For convenience, some lookaside patterns for popular distros are provided via the `--cdn` option.  You can specify this without needing to use the longer `--cdn-url`.  For example, when importing from CentOS 9 Stream, you could use `--cdn centos-stream`





