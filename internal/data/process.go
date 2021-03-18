package data

import (
	"git.rockylinux.org/release-engineering/public/srpmproc/internal/blob"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

type ProcessData struct {
	RpmLocation        string
	UpstreamPrefix     string
	SshKeyLocation     string
	SshUser            string
	Version            int
	GitCommitterName   string
	GitCommitterEmail  string
	Mode               int
	ModulePrefix       string
	ImportBranchPrefix string
	BranchPrefix       string
	SingleTag          string
	Authenticator      *ssh.PublicKeys
	Importer           ImportMode
	BlobStorage        blob.Storage
	NoDupMode          bool
	ModuleMode         bool
	TmpFsMode          string
	NoStorageDownload  bool
	NoStorageUpload    bool
	FsCreator          func(branch string) billy.Filesystem
}
