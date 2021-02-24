package directives

import (
	"git.rockylinux.org/release-engineering/public/srpmproc/internal/data"
	srpmprocpb "git.rockylinux.org/release-engineering/public/srpmproc/pb"
	"github.com/go-git/go-git/v5"
)

func lookaside(cfg *srpmprocpb.Cfg, _ *data.ProcessData, _ *data.ModeData, _ *git.Worktree, pushTree *git.Worktree) error {
	return nil
}
