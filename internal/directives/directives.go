package directives

import (
	"encoding/json"
	srpmprocpb "git.rockylinux.org/release-engineering/public/srpmproc/pb"
	"github.com/go-git/go-git/v5"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func checkAddPrefix(file string) string {
	if strings.HasPrefix(file, "SOURCES/") ||
		strings.HasPrefix(file, "SPECS/") {
		return file
	}

	return filepath.Join("SOURCES", file)
}

func Apply(cfg *srpmprocpb.Cfg, patchTree *git.Worktree, pushTree *git.Worktree) {
	var errs []string

	err := replace(cfg, patchTree, pushTree)
	if err != nil {
		errs = append(errs, err.Error())
	}

	err = del(cfg, patchTree, pushTree)
	if err != nil {
		errs = append(errs, err.Error())
	}

	err = add(cfg, patchTree, pushTree)
	if err != nil {
		errs = append(errs, err.Error())
	}

	err = patch(cfg, patchTree, pushTree)
	if err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		err := json.NewEncoder(os.Stdout).Encode(errs)
		if err != nil {
			log.Fatal(errs)
		}

		os.Exit(1)
	}
}
