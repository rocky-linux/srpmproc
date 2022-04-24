# srpmproc
Upstream package importer with auto patching. Reference implementation for OpenPatch

# Usage
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
      --cdn-url string                  CDN URL to download blobs from (default "https://git.centos.org/sources")
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
      --rpm-prefix string               Where to retrieve SRPM content. Only used when source-rpm is not a local file (default "https://git.centos.org/rpms")
      --single-tag string               If set, only this tag is imported
      --source-rpm string               Location of RPM to process
      --ssh-key-location string         Location of the SSH key to use to authenticate against upstream
      --ssh-user string                 SSH User (default "git")
      --storage-addr string             Bucket to use as blob storage
      --strict-branch-mode              If enabled, only branches with the calculated name are imported and not prefix only
      --tmpfs-mode string               If set, packages are imported to path and patched but not pushed
      --upstream-prefix string          Upstream git repository prefix
      --version int                     Upstream version

Use "srpmproc [command] --help" for more information about a command.
```
