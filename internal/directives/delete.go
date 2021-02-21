package directives

import (
	"errors"
	"fmt"
	srpmprocpb "git.rockylinux.org/release-engineering/public/srpmproc/pb"
	"github.com/go-git/go-git/v5"
)

func del(cfg *srpmprocpb.Cfg, _ *git.Worktree, pushTree *git.Worktree) error {
	for _, del := range cfg.Delete {
		filePath := del.File
		_, err := pushTree.Filesystem.Stat(filePath)
		if err != nil {
			return errors.New(fmt.Sprintf("FILE_DOES_NOT_EXIST:%s", filePath))
		}

		err = pushTree.Filesystem.Remove(filePath)
		if err != nil {
			return errors.New(fmt.Sprintf("COULD_NOT_DELETE_FILE:%s", filePath))
		}
	}

	return nil
}
