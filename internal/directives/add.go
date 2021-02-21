package directives

import (
	"errors"
	"fmt"
	srpmprocpb "git.rockylinux.org/release-engineering/public/srpmproc/pb"
	"github.com/go-git/go-git/v5"
	"io/ioutil"
	"os"
	"path/filepath"
)

func add(cfg *srpmprocpb.Cfg, patchTree *git.Worktree, pushTree *git.Worktree) error {
	for _, add := range cfg.Add {
		filePath := checkAddPrefix(filepath.Base(add.File))

		f, err := pushTree.Filesystem.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return errors.New(fmt.Sprintf("COULD_NOT_OPEN_DESTINATION:%s", filePath))
		}

		fPatch, err := patchTree.Filesystem.OpenFile(add.File, os.O_RDONLY, 0644)
		if err != nil {
			return errors.New(fmt.Sprintf("COULD_NOT_OPEN_FROM:%s", add.File))
		}

		replacingBytes, err := ioutil.ReadAll(fPatch)
		if err != nil {
			return errors.New(fmt.Sprintf("COULD_NOT_READ_FROM:%s", add.File))
		}

		_, err = f.Write(replacingBytes)
		if err != nil {
			return errors.New(fmt.Sprintf("COULD_NOT_WRITE_DESTIONATION:%s", filePath))
		}
	}

	return nil
}
