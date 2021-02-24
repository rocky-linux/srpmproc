package data

import (
	"github.com/cavaliercoder/go-rpm"
	"github.com/go-git/go-git/v5"
	"hash"
)

type ImportMode interface {
	RetrieveSource(pd *ProcessData) *ModeData
	WriteSource(pd *ProcessData, md *ModeData)
	PostProcess(md *ModeData)
	ImportName(pd *ProcessData, md *ModeData) string
}

type ModeData struct {
	Repo            *git.Repository
	Worktree        *git.Worktree
	RpmFile         *rpm.PackageFile
	FileWrites      map[string][]byte
	TagBranch       string
	PushBranch      string
	Branches        []string
	SourcesToIgnore []*IgnoredSource
	BlobCache       map[string][]byte
}

type IgnoredSource struct {
	Name         string
	HashFunction hash.Hash
	Expired      bool
}
