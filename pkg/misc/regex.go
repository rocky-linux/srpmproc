package misc

import (
	"fmt"
	"regexp"
)

func GetTagImportRegex(importBranchPrefix string, allowStreamBranches bool) *regexp.Regexp {
	if allowStreamBranches {
		return regexp.MustCompile(fmt.Sprintf("refs/tags/(imports/(%s(?:.s|.)|%s(?:|s).+)/(.*))", importBranchPrefix, importBranchPrefix))
	} else {
		return regexp.MustCompile(fmt.Sprintf("refs/tags/(imports/(%s.|%s.-.+)/(.*))", importBranchPrefix, importBranchPrefix))
	}
}
