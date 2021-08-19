// Copyright (c) 2021 The Srpmproc Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package srpmproc

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/rocky-linux/srpmproc/pkg/data"
)

var tagImportRegex *regexp.Regexp

func gitlabify(str string) string {
	if str == "tree" {
		return "treepkg"
	}

	return strings.Replace(str, "+", "plus", -1)
}

// ProcessRPM checks the RPM specs and discards any remote files
// This functions also sorts files into directories
// .spec files goes into -> SPECS
// metadata files goes to root
// source files goes into -> SOURCES
// all files that are remote goes into .gitignore
// all ignored files' hash goes into .{Name}.metadata
func ProcessRPM(pd *data.ProcessData) {
	if pd.AllowStreamBranches {
		tagImportRegex = regexp.MustCompile(fmt.Sprintf("refs/tags/(imports/(%s(?:.s|.)|%s(?:|s).+)/(.*))", pd.ImportBranchPrefix, pd.ImportBranchPrefix))
	} else {
		tagImportRegex = regexp.MustCompile(fmt.Sprintf("refs/tags/(imports/(%s.|%s.-.+)/(.*))", pd.ImportBranchPrefix, pd.ImportBranchPrefix))
	}

	md := pd.Importer.RetrieveSource(pd)
	md.BlobCache = map[string][]byte{}

	remotePrefix := "rpms"
	if pd.ModuleMode {
		remotePrefix = "modules"
	}

	latestHashForBranch := map[string]string{}

	// already uploaded blobs are skipped
	var alreadyUploadedBlobs []string

	// if no-dup-mode is enabled then skip already imported versions
	var tagIgnoreList []string
	if pd.NoDupMode {
		repo, err := git.Init(memory.NewStorage(), memfs.New())
		if err != nil {
			log.Fatalf("could not init git repo: %v", err)
		}
		remoteUrl := fmt.Sprintf("%s/%s/%s.git", pd.UpstreamPrefix, remotePrefix, gitlabify(md.RpmFile.Name()))
		refspec := config.RefSpec("+refs/heads/*:refs/remotes/origin/*")

		remote, err := repo.CreateRemote(&config.RemoteConfig{
			Name:  "origin",
			URLs:  []string{remoteUrl},
			Fetch: []config.RefSpec{refspec},
		})
		if err != nil {
			log.Fatalf("could not create remote: %v", err)
		}

		list, err := remote.List(&git.ListOptions{
			Auth: pd.Authenticator,
		})
		if err != nil {
			log.Println("ignoring no-dup-mode")
		} else {
			for _, ref := range list {
				if !strings.HasPrefix(string(ref.Name()), "refs/tags/imports") {
					continue
				}
				tagIgnoreList = append(tagIgnoreList, string(ref.Name()))
			}
		}
	}

	sourceRepo := *md.Repo
	sourceWorktree := *md.Worktree

	commitPin := map[string]string{}

	if pd.SingleTag != "" {
		md.Branches = []string{fmt.Sprintf("refs/tags/%s", pd.SingleTag)}
	} else if len(pd.ManualCommits) > 0 {
		md.Branches = []string{}
		for _, commit := range pd.ManualCommits {
			branchCommit := strings.Split(commit, ":")
			if len(branchCommit) != 2 {
				log.Fatalln("invalid manual commit list")
			}

			head := fmt.Sprintf("refs/tags/imports/%s/%s-%s", branchCommit[0], md.RpmFile.Name(), branchCommit[1])
			md.Branches = append(md.Branches, head)
			commitPin[head] = branchCommit[1]
		}
	}

	for _, branch := range md.Branches {
		md.Repo = &sourceRepo
		md.Worktree = &sourceWorktree
		md.TagBranch = branch
		for _, source := range md.SourcesToIgnore {
			source.Expired = true
		}

		if strings.Contains(md.TagBranch, "-beta") {
			continue
		}

		var matchString string
		if !tagImportRegex.MatchString(md.TagBranch) {
			if pd.ModuleMode {
				prefix := fmt.Sprintf("refs/heads/%s%d", pd.ImportBranchPrefix, pd.Version)
				if strings.HasPrefix(md.TagBranch, prefix) {
					replace := strings.Replace(md.TagBranch, "refs/heads/", "", 1)
					matchString = fmt.Sprintf("refs/tags/imports/%s/%s", replace, filepath.Base(pd.RpmLocation))
					log.Printf("using match string: %s", matchString)
				}
			}
			if !tagImportRegex.MatchString(matchString) {
				continue
			}
		} else {
			matchString = md.TagBranch
		}

		match := tagImportRegex.FindStringSubmatch(matchString)
		md.PushBranch = pd.BranchPrefix + strings.TrimPrefix(match[2], pd.ImportBranchPrefix)
		newTag := "imports/" + pd.BranchPrefix + strings.TrimPrefix(match[1], "imports/"+pd.ImportBranchPrefix)
		newTag = strings.Replace(newTag, "%", "_", -1)

		rpmFile := md.RpmFile
		// create new Repo for final dist
		repo, err := git.Init(memory.NewStorage(), pd.FsCreator(md.PushBranch))
		if err != nil {
			log.Fatalf("could not create new dist Repo: %v", err)
		}
		w, err := repo.Worktree()
		if err != nil {
			log.Fatalf("could not get dist Worktree: %v", err)
		}

		shouldContinue := true
		for _, ignoredTag := range tagIgnoreList {
			if ignoredTag == "refs/tags/"+newTag {
				log.Printf("skipping %s", ignoredTag)
				shouldContinue = false
			}
		}
		if !shouldContinue {
			continue
		}

		// create a new remote
		remoteUrl := fmt.Sprintf("%s/%s/%s.git", pd.UpstreamPrefix, remotePrefix, gitlabify(rpmFile.Name()))
		log.Printf("using remote: %s", remoteUrl)
		refspec := config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", md.PushBranch, md.PushBranch))
		log.Printf("using refspec: %s", refspec)

		_, err = repo.CreateRemote(&config.RemoteConfig{
			Name:  "origin",
			URLs:  []string{remoteUrl},
			Fetch: []config.RefSpec{refspec},
		})
		if err != nil {
			log.Fatalf("could not create remote: %v", err)
		}

		err = repo.Fetch(&git.FetchOptions{
			RemoteName: "origin",
			RefSpecs:   []config.RefSpec{refspec},
			Auth:       pd.Authenticator,
		})

		refName := plumbing.NewBranchReferenceName(md.PushBranch)
		log.Printf("set reference to ref: %s", refName)

		var hash plumbing.Hash
		if commitPin[md.PushBranch] != "" {
			hash = plumbing.NewHash(commitPin[md.PushBranch])
		}

		if err != nil {
			h := plumbing.NewSymbolicReference(plumbing.HEAD, refName)
			if err := repo.Storer.CheckAndSetReference(h, nil); err != nil {
				log.Fatalf("could not set reference: %v", err)
			}
		} else {
			err = w.Checkout(&git.CheckoutOptions{
				Branch: plumbing.NewRemoteReferenceName("origin", md.PushBranch),
				Hash:   hash,
				Force:  true,
			})
			if err != nil {
				log.Fatalf("could not checkout: %v", err)
			}
		}

		pd.Importer.WriteSource(pd, md)

		data.CopyFromFs(md.Worktree.Filesystem, w.Filesystem, ".")
		md.Repo = repo
		md.Worktree = w

		if pd.ModuleMode {
			patchModuleYaml(pd, md)
		} else {
			executePatchesRpm(pd, md)
		}

		// get ignored files hash and add to .{Name}.metadata
		metadataFile := fmt.Sprintf(".%s.metadata", rpmFile.Name())
		metadata, err := w.Filesystem.Create(metadataFile)
		if err != nil {
			log.Fatalf("could not create metadata file: %v", err)
		}
		for _, source := range md.SourcesToIgnore {
			sourcePath := source.Name

			_, err := w.Filesystem.Stat(sourcePath)
			if source.Expired || err != nil {
				continue
			}

			sourceFile, err := w.Filesystem.Open(sourcePath)
			if err != nil {
				log.Fatalf("could not open ignored source file %s: %v", sourcePath, err)
			}
			sourceFileBts, err := ioutil.ReadAll(sourceFile)
			if err != nil {
				log.Fatalf("could not read the whole of ignored source file: %v", err)
			}

			source.HashFunction.Reset()
			_, err = source.HashFunction.Write(sourceFileBts)
			if err != nil {
				log.Fatalf("could not write bytes to hash function: %v", err)
			}
			checksum := hex.EncodeToString(source.HashFunction.Sum(nil))
			checksumLine := fmt.Sprintf("%s %s\n", checksum, sourcePath)
			_, err = metadata.Write([]byte(checksumLine))
			if err != nil {
				log.Fatalf("could not write to metadata file: %v", err)
			}

			if data.StrContains(alreadyUploadedBlobs, checksum) {
				continue
			}
			if !pd.BlobStorage.Exists(checksum) && !pd.NoStorageUpload {
				pd.BlobStorage.Write(checksum, sourceFileBts)
				log.Printf("wrote %s to blob storage", checksum)
			}
			alreadyUploadedBlobs = append(alreadyUploadedBlobs, checksum)
		}

		_, err = w.Add(metadataFile)
		if err != nil {
			log.Fatalf("could not add metadata file: %v", err)
		}

		lastFilesToAdd := []string{".gitignore", "SPECS"}
		for _, f := range lastFilesToAdd {
			_, err := w.Filesystem.Stat(f)
			if err == nil {
				_, err := w.Add(f)
				if err != nil {
					log.Fatalf("could not add %s: %v", f, err)
				}
			}
		}

		if pd.TmpFsMode != "" {
			continue
		}

		pd.Importer.PostProcess(md)

		// show status
		status, _ := w.Status()
		log.Printf("successfully processed:\n%s", status)

		statusLines := strings.Split(status.String(), "\n")
		for _, line := range statusLines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "D") {
				path := strings.TrimPrefix(trimmed, "D ")
				_, err := w.Remove(path)
				if err != nil {
					log.Fatalf("could not delete extra file %s: %v", path, err)
				}
			}
		}

		var hashes []plumbing.Hash
		var pushRefspecs []config.RefSpec

		head, err := repo.Head()
		if err != nil {
			hashes = nil
			pushRefspecs = append(pushRefspecs, "*:*")
		} else {
			log.Printf("tip %s", head.String())
			hashes = append(hashes, head.Hash())
			refOrigin := "refs/heads/" + md.PushBranch
			pushRefspecs = append(pushRefspecs, config.RefSpec(fmt.Sprintf("HEAD:%s", refOrigin)))
		}

		// we are now finished with the tree and are going to push it to the src Repo
		// create import commit
		commit, err := w.Commit("import "+pd.Importer.ImportName(pd, md), &git.CommitOptions{
			Author: &object.Signature{
				Name:  pd.GitCommitterName,
				Email: pd.GitCommitterEmail,
				When:  time.Now(),
			},
			Parents: hashes,
		})
		if err != nil {
			log.Fatalf("could not commit object: %v", err)
		}

		obj, err := repo.CommitObject(commit)
		if err != nil {
			log.Fatalf("could not get commit object: %v", err)
		}

		log.Printf("committed:\n%s", obj.String())

		_, err = repo.CreateTag(newTag, commit, &git.CreateTagOptions{
			Tagger: &object.Signature{
				Name:  pd.GitCommitterName,
				Email: pd.GitCommitterEmail,
				When:  time.Now(),
			},
			Message: "import " + md.TagBranch + " from " + pd.RpmLocation,
			SignKey: nil,
		})
		if err != nil {
			log.Fatalf("could not create tag: %v", err)
		}

		pushRefspecs = append(pushRefspecs, config.RefSpec("HEAD:"+plumbing.NewTagReferenceName(newTag)))

		err = repo.Push(&git.PushOptions{
			RemoteName: "origin",
			Auth:       pd.Authenticator,
			RefSpecs:   pushRefspecs,
			Force:      true,
		})
		if err != nil {
			log.Fatalf("could not push to remote: %v", err)
		}

		hashString := obj.Hash.String()
		latestHashForBranch[md.PushBranch] = hashString
	}

	err := json.NewEncoder(os.Stdout).Encode(latestHashForBranch)
	if err != nil {
		log.Fatalf("could not print hashes")
	}
}
