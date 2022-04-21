package misc

import (
	"fmt"
	"github.com/rocky-linux/srpmproc/pkg/data"
	"regexp"
)

func GetTagImportRegex(pd *data.ProcessData) *regexp.Regexp {
	branchRegex := fmt.Sprintf("%s%d%s", pd.ImportBranchPrefix, pd.Version, pd.BranchSuffix)
	if !pd.StrictBranchMode {
		branchRegex += ".+"
	}
	regex := fmt.Sprintf("refs/tags/(imports/(%s)/(.*))", branchRegex)

	return regexp.MustCompile(regex)
}
