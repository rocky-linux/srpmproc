package directives

import (
	"encoding/json"
	"git.rockylinux.org/release-engineering/public/srpmproc/internal/data"
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

func Apply(cfg *srpmprocpb.Cfg, pd *data.ProcessData, md *data.ModeData, patchTree *git.Worktree, pushTree *git.Worktree) {
	var errs []string

	directives := []func(*srpmprocpb.Cfg, *data.ProcessData, *data.ModeData, *git.Worktree, *git.Worktree) error{
		replace,
		del,
		add,
		patch,
		lookaside,
		specChange,
	}

	for _, directive := range directives {
		err := directive(cfg, pd, md, patchTree, pushTree)
		if err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		err := json.NewEncoder(os.Stdout).Encode(errs)
		if err != nil {
			log.Fatal(errs)
		}

		os.Exit(1)
	}
}
