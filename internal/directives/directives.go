package directives

import (
	"github.com/go-git/go-git/v5"
	srpmprocpb "github.com/mstg/srpmproc/pb"
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
	replace(cfg, patchTree, pushTree)
}
