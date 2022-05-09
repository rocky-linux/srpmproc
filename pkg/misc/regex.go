package misc

import (
	"fmt"
	"github.com/rocky-linux/srpmproc/pkg/data"
	"path/filepath"
	"regexp"
)

func GetTagImportRegex(pd *data.ProcessData) *regexp.Regexp {
	branchRegex := fmt.Sprintf("%s%d%s", pd.ImportBranchPrefix, pd.Version, pd.BranchSuffix)
	if !pd.StrictBranchMode {
		branchRegex += "(?:.+|)"
	} else {
		branchRegex += "(?:-stream-.+|)"
	}

	initialVerRegex := filepath.Base(pd.RpmLocation) + "-"
	if pd.PackageVersion != "" {
		initialVerRegex += pd.PackageVersion + "-"
	} else {
		initialVerRegex += ".+-"
	}
	if pd.PackageRelease != "" {
		initialVerRegex += pd.PackageRelease
	} else {
		initialVerRegex += ".+"
	}

	regex := fmt.Sprintf("(?i)refs/tags/(imports/(%s)/(%s))", branchRegex, initialVerRegex)

	return regexp.MustCompile(regex)
}
