package directives

import (
	"errors"
	"fmt"
	srpmprocpb "git.rockylinux.org/release-engineering/public/srpmproc/pb"
	"github.com/go-git/go-git/v5"
	"io/ioutil"
	"os"
)

func replace(cfg *srpmprocpb.Cfg, patchTree *git.Worktree, pushTree *git.Worktree) error {
	for _, replace := range cfg.Replace {
		filePath := checkAddPrefix(replace.File)
		stat, err := pushTree.Filesystem.Stat(filePath)
		if replace.File == "" || err != nil {
			return errors.New(fmt.Sprintf("INVALID_FILE:%s", filePath))
		}

		err = pushTree.Filesystem.Remove(filePath)
		if err != nil {
			return errors.New(fmt.Sprintf("COULD_NOT_REMOVE_OLD_FILE:%s", filePath))
		}

		f, err := pushTree.Filesystem.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, stat.Mode())
		if err != nil {
			return errors.New(fmt.Sprintf("COULD_NOT_OPEN_REPLACEMENT:%s", filePath))
		}

		switch replacing := replace.Replacing.(type) {
		case *srpmprocpb.Replace_WithFile:
			fPatch, err := patchTree.Filesystem.OpenFile(replacing.WithFile, os.O_RDONLY, 0644)
			if err != nil {
				return errors.New(fmt.Sprintf("COULD_NOT_OPEN_REPLACING:%s", replacing.WithFile))
			}

			replacingBytes, err := ioutil.ReadAll(fPatch)
			if err != nil {
				return errors.New(fmt.Sprintf("COULD_NOT_READ_REPLACING:%s", replacing.WithFile))
			}

			_, err = f.Write(replacingBytes)
			if err != nil {
				return errors.New(fmt.Sprintf("COULD_NOT_WRITE_REPLACING:%s", replacing.WithFile))
			}
			break
		case *srpmprocpb.Replace_WithInline:
			_, err := f.Write([]byte(replacing.WithInline))
			if err != nil {
				return errors.New(fmt.Sprintf("COULD_NOT_WRITE_INLINE:%s", filePath))
			}
			break
		}
	}

	return nil
}
