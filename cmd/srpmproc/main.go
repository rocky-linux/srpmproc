package main

import (
	"fmt"
	"git.rockylinux.org/release-engineering/public/srpmproc/internal/blob"
	"git.rockylinux.org/release-engineering/public/srpmproc/internal/blob/file"
	"git.rockylinux.org/release-engineering/public/srpmproc/internal/blob/gcs"
	"git.rockylinux.org/release-engineering/public/srpmproc/internal/blob/s3"
	"git.rockylinux.org/release-engineering/public/srpmproc/internal/data"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"git.rockylinux.org/release-engineering/public/srpmproc/internal"
	"github.com/spf13/cobra"
)

var (
	sourceRpm          string
	sshKeyLocation     string
	sshUser            string
	upstreamPrefix     string
	version            int
	storageAddr        string
	gitCommitterName   string
	gitCommitterEmail  string
	modulePrefix       string
	rpmPrefix          string
	importBranchPrefix string
	branchPrefix       string
	singleTag          string
	noDupMode          bool
	moduleMode         bool
	tmpFsMode          bool
	noStorageDownload  bool
	noStorageUpload    bool
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
	// create ssh key authenticator
	authenticator, err := ssh.NewPublicKeysFromFile(sshUser, lastKeyLocation, "")
	if err != nil {
		log.Fatalf("could not get git authenticator: %v", err)
	}

	fsCreator := func() billy.Filesystem {
		return memfs.New()
	}

	if tmpFsMode {
		tmpDir, err := ioutil.TempDir(os.TempDir(), "srpmproc-*")
		if err != nil {
			log.Fatalf("could not create temp dir: %v", err)
		}
		log.Printf("using temp dir: %s", tmpDir)
		fsCreator = func() billy.Filesystem {
			return osfs.New(tmpDir)
		}
	}

	internal.ProcessRPM(&data.ProcessData{
		Importer:           importer,
		RpmLocation:        sourceRpmLocation,
		UpstreamPrefix:     upstreamPrefix,
		SshKeyLocation:     sshKeyLocation,
		SshUser:            sshUser,
		Version:            version,
		BlobStorage:        blobStorage,
		GitCommitterName:   gitCommitterName,
		GitCommitterEmail:  gitCommitterEmail,
		ModulePrefix:       modulePrefix,
		ImportBranchPrefix: importBranchPrefix,
		BranchPrefix:       branchPrefix,
		SingleTag:          singleTag,
		Authenticator:      authenticator,
		NoDupMode:          noDupMode,
		ModuleMode:         moduleMode,
		TmpFsMode:          tmpFsMode,
		NoStorageDownload:  noStorageDownload,
		NoStorageUpload:    noStorageUpload,
		FsCreator:          fsCreator,
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
	root.Flags().BoolVar(&tmpFsMode, "tmpfs-mode", false, "If enabled, packages are imported and patched but not pushed")
	root.Flags().BoolVar(&noStorageDownload, "no-storage-download", false, "If enabled, blobs are always downloaded from upstream")
	root.Flags().BoolVar(&noStorageUpload, "no-storage-upload", false, "If enabled, blobs are not uploaded to blob storage")

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
