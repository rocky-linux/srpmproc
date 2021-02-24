package directives

import (
	"bytes"
	"errors"
	"fmt"
	"git.rockylinux.org/release-engineering/public/srpmproc/internal/data"
	srpmprocpb "git.rockylinux.org/release-engineering/public/srpmproc/pb"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/go-git/go-git/v5"
	"log"
)

func patch(cfg *srpmprocpb.Cfg, _ *data.ProcessData, _ *data.ModeData, patchTree *git.Worktree, pushTree *git.Worktree) error {
	for _, patch := range cfg.Patch {
		patchFile, err := patchTree.Filesystem.Open(patch.File)
		if err != nil {
			return errors.New(fmt.Sprintf("COULD_NOT_OPEN_PATCH_FILE:%s", patch.File))
		}
		files, _, err := gitdiff.Parse(patchFile)
		if err != nil {
			log.Printf("could not parse patch file: %v", err)
			return errors.New(fmt.Sprintf("COULD_NOT_PARSE_PATCH_FILE:%s", patch.File))
		}

		for _, patchedFile := range files {
			srcPath := patchedFile.NewName
			if !patch.Strict {
				srcPath = checkAddPrefix(patchedFile.NewName)
			}
			var output bytes.Buffer
			if !patchedFile.IsDelete && !patchedFile.IsNew {
				patchSubjectFile, err := pushTree.Filesystem.Open(srcPath)
				if err != nil {
					return errors.New(fmt.Sprintf("COULD_NOT_OPEN_PATCH_SUBJECT:%s", srcPath))
				}

				err = gitdiff.NewApplier(patchSubjectFile).ApplyFile(&output, patchedFile)
				if err != nil {
					log.Printf("could not apply patch: %v", err)
					return errors.New(fmt.Sprintf("COULD_NOT_APPLY_PATCH_WITH_SUBJECT:%s", srcPath))
				}
			}

			oldName := patchedFile.OldName
			if !patch.Strict {
				oldName = checkAddPrefix(patchedFile.OldName)
			}
			_ = pushTree.Filesystem.Remove(oldName)
			_ = pushTree.Filesystem.Remove(srcPath)

			if patchedFile.IsNew {
				newFile, err := pushTree.Filesystem.Create(srcPath)
				if err != nil {
					return errors.New(fmt.Sprintf("COULD_NOT_CREATE_NEW_FILE:%s", srcPath))
				}
				err = gitdiff.NewApplier(newFile).ApplyFile(&output, patchedFile)
				if err != nil {
					return errors.New(fmt.Sprintf("COULD_NOT_APPLY_PATCH_TO_NEW_FILE:%s", srcPath))
				}
				_, err = newFile.Write(output.Bytes())
				if err != nil {
					return errors.New(fmt.Sprintf("COULD_NOT_WRITE_TO_NEW_FILE:%s", srcPath))
				}
				_, err = pushTree.Add(srcPath)
				if err != nil {
					return errors.New(fmt.Sprintf("COULD_NOT_ADD_NEW_FILE_TO_GIT:%s", srcPath))
				}
			} else if !patchedFile.IsDelete {
				newFile, err := pushTree.Filesystem.Create(srcPath)
				if err != nil {
					return errors.New(fmt.Sprintf("COULD_NOT_CREATE_POST_PATCH_FILE:%s", srcPath))
				}
				_, err = newFile.Write(output.Bytes())
				if err != nil {
					return errors.New(fmt.Sprintf("COULD_NOT_WRITE_POST_PATCH_FILE:%s", srcPath))
				}
				_, err = pushTree.Add(srcPath)
				if err != nil {
					return errors.New(fmt.Sprintf("COULD_NOT_ADD_POST_PATCH_FILE_TO_GIT:%s", srcPath))
				}
			} else {
				_, err = pushTree.Remove(oldName)
				if err != nil {
					return errors.New(fmt.Sprintf("COULD_NOT_REMOVE_FILE_FROM_GIT:%s", oldName))
				}
			}
		}
	}

	return nil
}
