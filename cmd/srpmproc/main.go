package main

import (
	"fmt"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/mstg/srpmproc/internal/blob"
	"github.com/mstg/srpmproc/internal/blob/gcs"
	"github.com/mstg/srpmproc/internal/blob/s3"
	"log"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/mstg/srpmproc/internal"
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
	noDupMode          bool
	moduleMode         bool
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

	var importer internal.ImportMode
	var blobStorage blob.Storage

	if strings.HasPrefix(storageAddr, "gs://") {
		blobStorage = gcs.New(strings.Replace(storageAddr, "gs://", "", 1))
	} else if strings.HasPrefix(storageAddr, "s3://") {
		blobStorage = s3.New(strings.Replace(storageAddr, "s3://", "", 1))
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

	internal.ProcessRPM(&internal.ProcessData{
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
		Authenticator:      authenticator,
		NoDupMode:          noDupMode,
		ModuleMode:         moduleMode,
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
	root.Flags().StringVar(&gitCommitterName, "git-committer-name", "distrobuild-bot", "Name of committer")
	root.Flags().StringVar(&gitCommitterEmail, "git-committer-email", "mustafa+distrobuild@bycrates.com", "Email of committer")
	root.Flags().StringVar(&modulePrefix, "module-prefix", "https://git.centos.org/modules", "Where to retrieve modules if exists. Only used when source-rpm is a git repo")
	root.Flags().StringVar(&rpmPrefix, "rpm-prefix", "https://git.centos.org/rpms", "Where to retrieve SRPM content. Only used when source-rpm is not a local file")
	root.Flags().StringVar(&importBranchPrefix, "import-branch-prefix", "c", "Import branch prefix")
	root.Flags().StringVar(&branchPrefix, "branch-prefix", "r", "Branch prefix (replaces import-branch-prefix)")
	root.Flags().BoolVar(&noDupMode, "no-dup-mode", false, "If enabled, skips already imported tags")
	root.Flags().BoolVar(&moduleMode, "module-mode", false, "If enabled, imports a module instead of a package")

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
