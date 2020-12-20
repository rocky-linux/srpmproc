package internal

import (
	"bytes"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/go-git/go-git/v5"
	"log"
	"path/filepath"
	"strings"
)

func srpmPatches(w *git.Worktree) {
	// check SRPM patches
	_, err := w.Filesystem.Stat("ROCKY/SRPM")
	if err == nil {
		// iterate through patches
		infos, err := w.Filesystem.ReadDir("ROCKY/SRPM")
		if err != nil {
			log.Fatalf("could not walk patches: %v", err)
		}

		for _, info := range infos {
			// can only process .patch files
			if !strings.HasSuffix(info.Name(), ".patch") {
				continue
			}

			log.Printf("applying %s", info.Name())
			filePath := filepath.Join("ROCKY/SRPM", info.Name())

			patch, err := w.Filesystem.Open(filePath)
			if err != nil {
				log.Fatalf("could not open patch file %s: %v", info.Name(), err)
			}
			files, _, err := gitdiff.Parse(patch)
			if err != nil {
				log.Fatalf("could not parse patch file: %v", err)
			}

			for _, patchedFile := range files {
				srcPath := patchedFile.NewName
				if !strings.HasPrefix(srcPath, "SPECS") {
					srcPath = filepath.Join("SOURCES", patchedFile.NewName)
				}
				var output bytes.Buffer
				if !patchedFile.IsDelete && !patchedFile.IsNew {
					patchSubjectFile, err := w.Filesystem.Open(srcPath)
					if err != nil {
						log.Fatalf("could not open patch subject: %v", err)
					}

					err = gitdiff.NewApplier(patchSubjectFile).ApplyFile(&output, patchedFile)
					if err != nil {
						log.Fatalf("could not apply patch: %v", err)
					}
				}

				oldName := filepath.Join("SOURCES", patchedFile.OldName)
				_ = w.Filesystem.Remove(oldName)
				_ = w.Filesystem.Remove(srcPath)

				if patchedFile.IsNew {
					newFile, err := w.Filesystem.Create(srcPath)
					if err != nil {
						log.Fatalf("could not create new file: %v", err)
					}
					err = gitdiff.NewApplier(newFile).ApplyFile(&output, patchedFile)
					if err != nil {
						log.Fatalf("could not apply patch: %v", err)
					}
					_, err = newFile.Write(output.Bytes())
					if err != nil {
						log.Fatalf("could not write post-patch file: %v", err)
					}
					_, err = w.Add(srcPath)
					if err != nil {
						log.Fatalf("could not add file %s to git: %v", srcPath, err)
					}
					log.Printf("git add %s", srcPath)
				} else if !patchedFile.IsDelete {
					newFile, err := w.Filesystem.Create(srcPath)
					if err != nil {
						log.Fatalf("could not create post-patch file: %v", err)
					}
					_, err = newFile.Write(output.Bytes())
					if err != nil {
						log.Fatalf("could not write post-patch file: %v", err)
					}
					_, err = w.Add(srcPath)
					if err != nil {
						log.Fatalf("could not add file %s to git: %v", srcPath, err)
					}
					log.Printf("git add %s", srcPath)
				} else {
					_, err = w.Remove(oldName)
					if err != nil {
						log.Fatalf("could not remove file %s to git: %v", oldName, err)
					}
					log.Printf("git rm %s", oldName)
				}
			}

			_, err = w.Add(filePath)
			if err != nil {
				log.Fatalf("could not add file %s to git: %v", filePath, err)
			}
			log.Printf("git add %s", filePath)
		}
	}
}

func executePatches(w *git.Worktree) {
	// check if patches exist
	_, err := w.Filesystem.Stat("ROCKY")
	if err == nil {
		srpmPatches(w)
	}
}
