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
	"fmt"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/rocky-linux/srpmproc/internal/blob"
	"github.com/rocky-linux/srpmproc/internal/blob/file"
	"github.com/rocky-linux/srpmproc/internal/blob/gcs"
	"github.com/rocky-linux/srpmproc/internal/blob/s3"
	"github.com/rocky-linux/srpmproc/internal/data"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/rocky-linux/srpmproc/internal"
	"github.com/spf13/cobra"
)

var (
	sourceRpm            string
	sshKeyLocation       string
	sshUser              string
	upstreamPrefix       string
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
	upstreamPrefixHttps  string
	moduleFallbackStream string
)

var root = &cobra.Command{
	Use: "srpmproc",
	Run: mn,
}

func mn(_ *cobra.Command, _ []string) {
	switch version {
	case 8:
		break
	default:
		log.Fatalf("unsupported upstream version %d", version)
	}

	var importer data.ImportMode
	var blobStorage blob.Storage

	if strings.HasPrefix(storageAddr, "gs://") {
		blobStorage = gcs.New(strings.Replace(storageAddr, "gs://", "", 1))
	} else if strings.HasPrefix(storageAddr, "s3://") {
		blobStorage = s3.New(strings.Replace(storageAddr, "s3://", "", 1))
	} else if strings.HasPrefix(storageAddr, "file://") {
		blobStorage = file.New(strings.Replace(storageAddr, "file://", "", 1))
	} else {
		log.Fatalf("invalid blob storage")
	}

	sourceRpmLocation := ""
	if strings.HasPrefix(sourceRpm, "file://") {
		sourceRpmLocation = strings.TrimPrefix(sourceRpm, "file://")
		importer = &internal.SrpmMode{}
	} else {
		if moduleMode {
			sourceRpmLocation = fmt.Sprintf("%s/%s", modulePrefix, sourceRpm)
		} else {
			sourceRpmLocation = fmt.Sprintf("%s/%s", rpmPrefix, sourceRpm)
		}
		importer = &internal.GitMode{}
	}

	lastKeyLocation := sshKeyLocation
	if lastKeyLocation == "" {
		usr, err := user.Current()
		if err != nil {
			log.Fatalf("could not get user: %v", err)
		}
		lastKeyLocation = filepath.Join(usr.HomeDir, ".ssh/id_rsa")
	}

	var authenticator *ssh.PublicKeys

	if tmpFsMode == "" {
		var err error
		// create ssh key authenticator
		authenticator, err = ssh.NewPublicKeysFromFile(sshUser, lastKeyLocation, "")
		if err != nil {
			log.Fatalf("could not get git authenticator: %v", err)
		}
	}

	fsCreator := func(branch string) billy.Filesystem {
		return memfs.New()
	}

	if tmpFsMode != "" {
		log.Printf("using tmpfs dir: %s", tmpFsMode)
		fsCreator = func(branch string) billy.Filesystem {
			tmpDir := filepath.Join(tmpFsMode, branch)
			err := os.MkdirAll(tmpDir, 0755)
			if err != nil {
				log.Fatalf("could not create tmpfs dir: %v", err)
			}
			return osfs.New(tmpDir)
		}
	}

	var manualCs []string
	if strings.TrimSpace(manualCommits) != "" {
		manualCs = strings.Split(manualCommits, ",")
	}

	internal.ProcessRPM(&data.ProcessData{
		Importer:             importer,
		RpmLocation:          sourceRpmLocation,
		UpstreamPrefix:       upstreamPrefix,
		SshKeyLocation:       sshKeyLocation,
		SshUser:              sshUser,
		Version:              version,
		BlobStorage:          blobStorage,
		GitCommitterName:     gitCommitterName,
		GitCommitterEmail:    gitCommitterEmail,
		ModulePrefix:         modulePrefix,
		ImportBranchPrefix:   importBranchPrefix,
		BranchPrefix:         branchPrefix,
		SingleTag:            singleTag,
		Authenticator:        authenticator,
		NoDupMode:            noDupMode,
		ModuleMode:           moduleMode,
		TmpFsMode:            tmpFsMode,
		NoStorageDownload:    noStorageDownload,
		NoStorageUpload:      noStorageUpload,
		ManualCommits:        manualCs,
		UpstreamPrefixHttps:  upstreamPrefixHttps,
		ModuleFallbackStream: moduleFallbackStream,
		FsCreator:            fsCreator,
	})
}

func main() {
	root.Flags().StringVar(&sourceRpm, "source-rpm", "", "Location of RPM to process")
	_ = root.MarkFlagRequired("source-rpm")
	root.Flags().StringVar(&upstreamPrefix, "upstream-prefix", "", "Upstream git repository prefix")
	_ = root.MarkFlagRequired("upstream-prefix")
	root.Flags().IntVar(&version, "version", 0, "Upstream version")
	_ = root.MarkFlagRequired("version")
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
	root.Flags().StringVar(&singleTag, "single-tag", "", "If set, only this tag is imported")
	root.Flags().BoolVar(&noDupMode, "no-dup-mode", false, "If enabled, skips already imported tags")
	root.Flags().BoolVar(&moduleMode, "module-mode", false, "If enabled, imports a module instead of a package")
	root.Flags().StringVar(&tmpFsMode, "tmpfs-mode", "", "If set, packages are imported to path and patched but not pushed")
	root.Flags().BoolVar(&noStorageDownload, "no-storage-download", false, "If enabled, blobs are always downloaded from upstream")
	root.Flags().BoolVar(&noStorageUpload, "no-storage-upload", false, "If enabled, blobs are not uploaded to blob storage")
	root.Flags().StringVar(&manualCommits, "manual-commits", "", "Comma separated branch and commit list for packages with broken release tags (Format: BRANCH:HASH)")
	root.Flags().StringVar(&upstreamPrefixHttps, "upstream-prefix-https", "", "Web version of upstream prefix. Required if module-mode")
	root.Flags().StringVar(&moduleFallbackStream, "module-fallback-stream", "", "Override fallback stream. Some module packages are published as collections and mostly use the same stream name, some of them deviate from the main stream")

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
