package directives

import (
	"errors"
	"fmt"
	"git.rockylinux.org/release-engineering/public/srpmproc/internal/data"
	srpmprocpb "git.rockylinux.org/release-engineering/public/srpmproc/pb"
	"github.com/go-git/go-git/v5"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	sectionChangelog = "%changelog"
)

type sourcePatchOperationInLoopRequest struct {
	cfg           *srpmprocpb.Cfg
	field         string
	value         *string
	longestField  int
	lastNum       *int
	in            *bool
	expectedField string
	operation     srpmprocpb.SpecChange_FileOperation_Type
}

type sourcePatchOperationAfterLoopRequest struct {
	cfg           *srpmprocpb.Cfg
	inLoopNum     int
	lastNum       *int
	longestField  int
	newLines      *[]string
	in            *bool
	expectedField string
	operation     srpmprocpb.SpecChange_FileOperation_Type
}

func sourcePatchOperationInLoop(req *sourcePatchOperationInLoopRequest) error {
	if strings.HasPrefix(req.field, req.expectedField) {
		for _, file := range req.cfg.SpecChange.File {
			if file.Type != req.operation {
				continue
			}

			switch file.Mode.(type) {
			case *srpmprocpb.SpecChange_FileOperation_Delete:
				if file.Name == *req.value {
					*req.value = ""
				}
				break
			}
		}
		if req.field != req.expectedField {
			sourceNum, err := strconv.Atoi(strings.Split(req.field, req.expectedField)[1])
			if err != nil {
				return errors.New(fmt.Sprintf("INVALID_%s_NUM:%s", strings.ToUpper(req.expectedField), req.field))
			}
			*req.lastNum = sourceNum
		}
	}

	return nil
}

func sourcePatchOperationAfterLoop(req *sourcePatchOperationAfterLoopRequest) (bool, error) {
	if req.inLoopNum == *req.lastNum && *req.in {
		for _, file := range req.cfg.SpecChange.File {
			if file.Type != req.operation {
				continue
			}

			switch file.Mode.(type) {
			case *srpmprocpb.SpecChange_FileOperation_Add:
				field := fmt.Sprintf("%s%d", req.expectedField, *req.lastNum+1)
				spaces := calculateSpaces(req.longestField, len(field))
				*req.newLines = append(*req.newLines, fmt.Sprintf("%s:%s%s", field, spaces, file.Name))
				*req.lastNum++
				break
			}
		}
		*req.in = false

		return true, nil
	}

	return false, nil
}

func calculateSpaces(longestField int, fieldLength int) string {
	return strings.Repeat(" ", longestField+8-fieldLength)
}

func searchAndReplaceLine(line string, sar []*srpmprocpb.SpecChange_SearchAndReplaceOperation) string {
	for _, searchAndReplace := range sar {
		switch searchAndReplace.Identifier.(type) {
		case *srpmprocpb.SpecChange_SearchAndReplaceOperation_Any:
			line = strings.Replace(line, searchAndReplace.Find, searchAndReplace.Replace, int(searchAndReplace.N))
			break
		case *srpmprocpb.SpecChange_SearchAndReplaceOperation_StartsWith:
			if strings.HasPrefix(strings.TrimSpace(line), searchAndReplace.Find) {
				line = strings.Replace(line, searchAndReplace.Find, searchAndReplace.Replace, int(searchAndReplace.N))
			}
			break
		case *srpmprocpb.SpecChange_SearchAndReplaceOperation_EndsWith:
			if strings.HasSuffix(strings.TrimSpace(line), searchAndReplace.Find) {
				line = strings.Replace(line, searchAndReplace.Find, searchAndReplace.Replace, int(searchAndReplace.N))
			}
			break
		}
	}

	return line
}

func specChange(cfg *srpmprocpb.Cfg, pd *data.ProcessData, md *data.ModeData, _ *git.Worktree, pushTree *git.Worktree) error {
	// no spec change operations present
	// skip parsing spec
	if cfg.SpecChange == nil {
		return nil
	}

	specFiles, err := pushTree.Filesystem.ReadDir("SPECS")
	if err != nil {
		return errors.New("COULD_NOT_READ_SPECS_DIR")
	}

	if len(specFiles) != 1 {
		return errors.New("ONLY_ONE_SPEC_FILE_IS_SUPPORTED")
	}

	filePath := filepath.Join("SPECS", specFiles[0].Name())
	stat, err := pushTree.Filesystem.Stat(filePath)
	if err != nil {
		return errors.New("COULD_NOT_STAT_SPEC_FILE")
	}

	specFile, err := pushTree.Filesystem.OpenFile(filePath, os.O_RDONLY, 0644)
	if err != nil {
		return errors.New("COULD_NOT_READ_SPEC_FILE")
	}

	specBts, err := ioutil.ReadAll(specFile)
	if err != nil {
		return errors.New("COULD_NOT_READ_ALL_BYTES")
	}

	specStr := string(specBts)
	lines := strings.Split(specStr, "\n")

	var newLines []string
	futureAdditions := map[int]string{}
	lastSourceNum := 0
	lastPatchNum := 0
	inSources := false
	inPatches := false
	inSection := ""
	lastSource := ""
	lastPatch := ""

	version := ""
	importName := strings.Replace(pd.Importer.ImportName(pd, md), md.RpmFile.Name(), "1", 1)
	importNameSplit := strings.SplitN(importName, "-", 2)
	if len(importNameSplit) == 2 {
		versionSplit := strings.SplitN(importNameSplit[1], ".el", 2)
		if len(versionSplit) == 2 {
			version = versionSplit[0]
		} else {
			versionSplit := strings.SplitN(importNameSplit[1], ".module", 2)
			if len(versionSplit) == 2 {
				version = versionSplit[0]
			}
		}
	}

	fieldValueRegex := regexp.MustCompile("^[A-Z].+:")

	longestField := 0
	for _, line := range lines {
		if fieldValueRegex.MatchString(line) {
			fieldValue := strings.SplitN(line, ":", 2)
			field := strings.TrimSpace(fieldValue[0])
			longestField = int(math.Max(float64(len(field)), float64(longestField)))

			if strings.HasPrefix(field, "Source") {
				lastSource = field
			}

			if strings.HasPrefix(field, "Patch") {
				lastPatch = field
			}
		}
	}

	for lineNum, line := range lines {
		inLoopSourceNum := lastSourceNum
		inLoopPatchNum := lastPatchNum
		prefixLine := strings.TrimSpace(line)

		for i, addition := range futureAdditions {
			if lineNum == i {
				newLines = append(newLines, addition)
			}
		}

		if fieldValueRegex.MatchString(line) {
			line = searchAndReplaceLine(line, cfg.SpecChange.SearchAndReplace)
			fieldValue := strings.SplitN(line, ":", 2)
			field := strings.TrimSpace(fieldValue[0])
			value := strings.TrimSpace(fieldValue[1])

			if field == lastSource {
				inSources = true
			} else if field == lastPatch {
				inPatches = true
			}

			if field == "Version" && version == "" {
				version = value
			}

			for _, searchAndReplace := range cfg.SpecChange.SearchAndReplace {
				switch identifier := searchAndReplace.Identifier.(type) {
				case *srpmprocpb.SpecChange_SearchAndReplaceOperation_Field:
					if field == identifier.Field {
						value = strings.Replace(value, searchAndReplace.Find, searchAndReplace.Replace, int(searchAndReplace.N))
					}
					break
				}
			}

			for _, appendOp := range cfg.SpecChange.Append {
				if field == appendOp.Field {
					value = value + appendOp.Value

					if field == "Release" {
						version = version + appendOp.Value
					}
				}
			}

			spaces := calculateSpaces(longestField, len(field))

			err := sourcePatchOperationInLoop(&sourcePatchOperationInLoopRequest{
				cfg:           cfg,
				field:         field,
				value:         &value,
				lastNum:       &lastSourceNum,
				longestField:  longestField,
				in:            &inSources,
				expectedField: "Source",
				operation:     srpmprocpb.SpecChange_FileOperation_Source,
			})
			if err != nil {
				return err
			}

			err = sourcePatchOperationInLoop(&sourcePatchOperationInLoopRequest{
				cfg:           cfg,
				field:         field,
				value:         &value,
				longestField:  longestField,
				lastNum:       &lastPatchNum,
				in:            &inPatches,
				expectedField: "Patch",
				operation:     srpmprocpb.SpecChange_FileOperation_Patch,
			})
			if err != nil {
				return err
			}

			if value != "" {
				newLines = append(newLines, fmt.Sprintf("%s:%s%s", field, spaces, value))
			}
		} else {
			executed, err := sourcePatchOperationAfterLoop(&sourcePatchOperationAfterLoopRequest{
				cfg:           cfg,
				inLoopNum:     inLoopSourceNum,
				lastNum:       &lastSourceNum,
				longestField:  longestField,
				newLines:      &newLines,
				expectedField: "Source",
				in:            &inSources,
				operation:     srpmprocpb.SpecChange_FileOperation_Source,
			})
			if err != nil {
				return err
			}

			if executed && !strings.Contains(specStr, "Patch") {
				newLines = append(newLines, "")
				inPatches = true
			}

			executed, err = sourcePatchOperationAfterLoop(&sourcePatchOperationAfterLoopRequest{
				cfg:           cfg,
				inLoopNum:     inLoopPatchNum,
				lastNum:       &lastPatchNum,
				longestField:  longestField,
				newLines:      &newLines,
				expectedField: "Patch",
				in:            &inPatches,
				operation:     srpmprocpb.SpecChange_FileOperation_Patch,
			})
			if err != nil {
				return err
			}

			if executed && !strings.Contains(specStr, "%changelog") {
				newLines = append(newLines, "")
				newLines = append(newLines, "%changelog")
				inSection = sectionChangelog
			}

			if inSection == sectionChangelog {
				now := time.Now().Format("Mon Jan 02 2006")
				for _, changelog := range cfg.SpecChange.Changelog {
					newLines = append(newLines, fmt.Sprintf("* %s %s <%s> - %s", now, changelog.AuthorName, changelog.AuthorEmail, version))
					for _, msg := range changelog.Message {
						newLines = append(newLines, fmt.Sprintf("- %s", msg))
					}
					newLines = append(newLines, "")
				}
				inSection = ""
			} else {
				line = searchAndReplaceLine(line, cfg.SpecChange.SearchAndReplace)
			}

			if strings.HasPrefix(prefixLine, "%") {
				inSection = prefixLine

				for _, appendOp := range cfg.SpecChange.Append {
					if inSection == appendOp.Field {
						insertedLine := 0
						for i, x := range lines[lineNum+1:] {
							if strings.HasPrefix(strings.TrimSpace(x), "%") {
								insertedLine = lineNum + i + 2
								futureAdditions[insertedLine] = appendOp.Value
								break
							}
						}
						if insertedLine == 0 {
							for i, x := range lines[lineNum+1:] {
								if strings.TrimSpace(x) == "" {
									insertedLine = lineNum + i + 2
									futureAdditions[insertedLine] = appendOp.Value
									break
								}
							}
						}
					}
				}
			}

			newLines = append(newLines, line)
		}
	}

	err = pushTree.Filesystem.Remove(filePath)
	if err != nil {
		return errors.New(fmt.Sprintf("COULD_NOT_REMOVE_OLD_SPEC_FILE:%s", filePath))
	}

	f, err := pushTree.Filesystem.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, stat.Mode())
	if err != nil {
		return errors.New(fmt.Sprintf("COULD_NOT_OPEN_REPLACEMENT_SPEC_FILE:%s", filePath))
	}

	_, err = f.Write([]byte(strings.Join(newLines, "\n")))
	if err != nil {
		return errors.New("COULD_NOT_WRITE_NEW_SPEC_FILE")
	}

	return nil
}
