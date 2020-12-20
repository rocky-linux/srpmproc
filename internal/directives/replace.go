package directives

import (
	"github.com/go-git/go-git/v5"
	srpmprocpb "github.com/mstg/srpmproc/pb"
	"io/ioutil"
	"log"
	"os"
)

func replace(cfg *srpmprocpb.Cfg, patchTree *git.Worktree, pushTree *git.Worktree) {
	for _, replace := range cfg.Replace {
		filePath := checkAddPrefix(replace.File)
		stat, err := pushTree.Filesystem.Stat(filePath)
		if replace.File == "" || err != nil {
			log.Fatalf("file to replace is invalid")
		}

		err = pushTree.Filesystem.Remove(filePath)
		if err != nil {
			log.Fatalf("could not remove old file: %v", err)
		}

		f, err := pushTree.Filesystem.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, stat.Mode())
		if err != nil {
			log.Fatalf("could not open replacement file: %v", err)
		}

		switch replacing := replace.Replacing.(type) {
		case *srpmprocpb.Replace_WithFile:
			fPatch, err := patchTree.Filesystem.OpenFile(replacing.WithFile, os.O_RDONLY, 0644)
			if err != nil {
				log.Fatalf("could not open replacing file: %v", err)
			}

			replacingBytes, err := ioutil.ReadAll(fPatch)
			if err != nil {
				log.Fatalf("could not read replacing file: %v", err)
			}

			_, err = f.Write(replacingBytes)
			if err != nil {
				log.Fatalf("could not write replacing file: %v", err)
			}
			break
		case *srpmprocpb.Replace_WithInline:
			_, err := f.Write([]byte(replacing.WithInline))
			if err != nil {
				log.Fatalf("could not write inline replacement: %v", err)
			}
			break
		}
	}
}
