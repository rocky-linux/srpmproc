// Copyright (c) 2021 The Srpmproc Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/rocky-linux/srpmproc/pkg/srpmproc"

	"github.com/spf13/cobra"
)

var (
	sourceRpm            string
	sshKeyLocation       string
	sshUser              string
	upstreamPrefix       string
	upstreamVersion      int
	version              int
	storageAddr          string
	gitCommitterName     string
	gitCommitterEmail    string
	modulePrefix         string
	rpmPrefix            string
	importBranchPrefix   string
	branchPrefix         string
	singleTag            string
	noDupMode            bool
	moduleMode           bool
	tmpFsMode            string
	noStorageDownload    bool
	noStorageUpload      bool
	manualCommits        string
	moduleFallbackStream string
	branchSuffix         string
	strictBranchMode     bool
	basicUsername        string
	basicPassword        string
	packageVersion       string
	packageRelease       string
	taglessMode          bool
	cdn                  string
	moduleBranchNames    bool
)

var root = &cobra.Command{
	Use: "srpmproc",
	Run: mn,
}

func mn(_ *cobra.Command, _ []string) {
	pd, err := srpmproc.NewProcessData(&srpmproc.ProcessDataRequest{
		Version:              version,
		UpstreamVersion:      upstreamVersion,
		StorageAddr:          storageAddr,
		Package:              sourceRpm,
		ModuleMode:           moduleMode,
		TmpFsMode:            tmpFsMode,
		ModulePrefix:         modulePrefix,
		RpmPrefix:            rpmPrefix,
		SshKeyLocation:       sshKeyLocation,
		SshUser:              sshUser,
		ManualCommits:        manualCommits,
		UpstreamPrefix:       upstreamPrefix,
		GitCommitterName:     gitCommitterName,
		GitCommitterEmail:    gitCommitterEmail,
		ImportBranchPrefix:   importBranchPrefix,
		BranchPrefix:         branchPrefix,
		NoDupMode:            noDupMode,
		BranchSuffix:         branchSuffix,
		StrictBranchMode:     strictBranchMode,
		ModuleFallbackStream: moduleFallbackStream,
		NoStorageUpload:      noStorageUpload,
		NoStorageDownload:    noStorageDownload,
		SingleTag:            singleTag,
		CdnUrl:               cdnUrl,
		HttpUsername:         basicUsername,
		HttpPassword:         basicPassword,
		PackageVersion:       packageVersion,
		PackageRelease:       packageRelease,
		TaglessMode:          taglessMode,
		Cdn:                  cdn,
		ModuleBranchNames:    moduleBranchNames,
	})
	if err != nil {
		log.Fatal(err)
	}

	res, err := srpmproc.ProcessRPM(pd)
	if err != nil {
		log.Fatal(err)
	}

	err = json.NewEncoder(os.Stdout).Encode(res)
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	root.Flags().StringVar(&sourceRpm, "source-rpm", "", "Location of RPM to process")
	_ = root.MarkFlagRequired("source-rpm")
	root.Flags().StringVar(&upstreamPrefix, "upstream-prefix", "", "Upstream git repository prefix")
	_ = root.MarkFlagRequired("upstream-prefix")
	root.Flags().IntVar(&version, "version", 0, "Upstream version")
	_ = root.MarkFlagRequired("version")
	root.Flags().IntVar(&upstreamVersion, "upstream-version", 0, "Upstream version")

	root.Flags().StringVar(&storageAddr, "storage-addr", "", "Bucket to use as blob storage")
	_ = root.MarkFlagRequired("storage-addr")

	root.Flags().StringVar(&sshKeyLocation, "ssh-key-location", "", "Location of the SSH key to use to authenticate against upstream")
	root.Flags().StringVar(&sshUser, "ssh-user", "git", "SSH User")
	root.Flags().StringVar(&gitCommitterName, "git-committer-name", "rockyautomation", "Name of committer")
	root.Flags().StringVar(&gitCommitterEmail, "git-committer-email", "rockyautomation@rockylinux.org", "Email of committer")
	root.Flags().StringVar(&modulePrefix, "module-prefix", "https://git.centos.org/modules", "Where to retrieve modules if exists. Only used when source-rpm is a git repo")
	root.Flags().StringVar(&rpmPrefix, "rpm-prefix", "https://git.centos.org/rpms", "Where to retrieve SRPM content. Only used when source-rpm is not a local file")
	root.Flags().StringVar(&importBranchPrefix, "import-branch-prefix", "c", "Import branch prefix")
	root.Flags().StringVar(&branchPrefix, "branch-prefix", "r", "Branch prefix (replaces import-branch-prefix)")
	root.Flags().StringVar(&cdnUrl, "cdn-url", "https://git.centos.org/sources", "CDN URL to download blobs from. Simple URL follows default rocky/centos patterns. Can be customized using macros (see docs)")
	root.Flags().StringVar(&singleTag, "single-tag", "", "If set, only this tag is imported")
	root.Flags().BoolVar(&noDupMode, "no-dup-mode", false, "If enabled, skips already imported tags")
	root.Flags().BoolVar(&moduleMode, "module-mode", false, "If enabled, imports a module instead of a package")
	root.Flags().StringVar(&tmpFsMode, "tmpfs-mode", "", "If set, packages are imported to path and patched but not pushed")
	root.Flags().BoolVar(&noStorageDownload, "no-storage-download", false, "If enabled, blobs are always downloaded from upstream")
	root.Flags().BoolVar(&noStorageUpload, "no-storage-upload", false, "If enabled, blobs are not uploaded to blob storage")
	root.Flags().StringVar(&manualCommits, "manual-commits", "", "Comma separated branch and commit list for packages with broken release tags (Format: BRANCH:HASH)")
	root.Flags().StringVar(&moduleFallbackStream, "module-fallback-stream", "", "Override fallback stream. Some module packages are published as collections and mostly use the same stream name, some of them deviate from the main stream")
	root.Flags().StringVar(&branchSuffix, "branch-suffix", "", "Branch suffix to use for imported branches")
	root.Flags().BoolVar(&strictBranchMode, "strict-branch-mode", false, "If enabled, only branches with the calculated name are imported and not prefix only")
	root.Flags().StringVar(&basicUsername, "basic-username", "", "Basic auth username")
	root.Flags().StringVar(&basicPassword, "basic-password", "", "Basic auth password")
	root.Flags().StringVar(&packageVersion, "package-version", "", "Package version to fetch")
	root.Flags().StringVar(&packageRelease, "package-release", "", "Package release to fetch")
	root.Flags().BoolVar(&taglessMode, "taglessmode", false, "Tagless mode:  If set, pull the latest commit from the branch and determine version numbers from spec file.  This is auto-tried if tags aren't found.")
	root.Flags().StringVar(&cdn, "cdn", "", "CDN URL shortcuts for well-known distros, auto-assigns --cdn-url.  Valid values:  rocky8, rocky, fedora, centos, centos-stream.  Setting this overrides --cdn-url")
	root.Flags().BoolVar(&moduleBranchNames, "module-branch-names-only", false, "If enabled, module imports will use the branch name that is being imported, rather than use the commit hash.")

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
