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
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/rocky-linux/srpmproc/pkg/blob"
	"github.com/rocky-linux/srpmproc/pkg/blob/file"
	"github.com/rocky-linux/srpmproc/pkg/blob/gcs"
	"github.com/rocky-linux/srpmproc/pkg/blob/s3"
	"github.com/rocky-linux/srpmproc/pkg/modes"
	"io/ioutil"
	"log"
	"os"
	"os/user"
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

const (
	RpmPrefixCentOS     = "https://git.centos.org/rpms"
	ModulePrefixCentOS  = "https://git.centos.org/modules"
	RpmPrefixRocky      = "https://git.rockylinux.org/staging/rpms"
	ModulePrefixRocky   = "https://git.rockylinux.org/staging/modules"
	UpstreamPrefixRocky = "https://git.rockylinux.org/staging"
)

var tagImportRegex *regexp.Regexp

type ProcessDataRequest struct {
	// Required
	Version     int
	StorageAddr string
	Package     string

	// Optional
	ModuleMode           bool
	TmpFsMode            string
	ModulePrefix         string
	RpmPrefix            string
	SshKeyLocation       string
	SshUser              string
	ManualCommits        string
	UpstreamPrefix       string
	GitCommitterName     string
	GitCommitterEmail    string
	ImportBranchPrefix   string
	BranchPrefix         string
	FsCreator            data.FsCreatorFunc
	NoDupMode            bool
	AllowStreamBranches  bool
	ModuleFallbackStream string
	NoStorageUpload      bool
	NoStorageDownload    bool
	SingleTag            string
}

func gitlabify(str string) string {
	if str == "tree" {
		return "treepkg"
	}

	return strings.Replace(str, "+", "plus", -1)
}

func NewProcessData(req *ProcessDataRequest) (*data.ProcessData, error) {
	switch req.Version {
	case 8:
		break
	default:
		return nil, fmt.Errorf("unsupported upstream version %d", req.Version)
	}

	// Set defaults
	if req.ModulePrefix == "" {
		req.ModulePrefix = ModulePrefixCentOS
	}
	if req.RpmPrefix == "" {
		req.RpmPrefix = RpmPrefixCentOS
	}
	if req.SshUser == "" {
		req.SshUser = "git"
	}
	if req.UpstreamPrefix == "" {
		req.UpstreamPrefix = UpstreamPrefixRocky
	}
	if req.GitCommitterName == "" {
		req.GitCommitterName = "rockyautomation"
	}
	if req.GitCommitterEmail == "" {
		req.GitCommitterEmail = "rockyautomation@rockylinux.org"
	}
	if req.ImportBranchPrefix == "" {
		req.ImportBranchPrefix = "c"
	}
	if req.BranchPrefix == "" {
		req.BranchPrefix = "r"
	}

	// Validate required
	if req.Package == "" {
		return nil, fmt.Errorf("package cannot be empty")
	}

	var importer data.ImportMode
	var blobStorage blob.Storage

	if strings.HasPrefix(req.StorageAddr, "gs://") {
		blobStorage = gcs.New(strings.Replace(req.StorageAddr, "gs://", "", 1))
	} else if strings.HasPrefix(req.StorageAddr, "s3://") {
		blobStorage = s3.New(strings.Replace(req.StorageAddr, "s3://", "", 1))
	} else if strings.HasPrefix(req.StorageAddr, "file://") {
		blobStorage = file.New(strings.Replace(req.StorageAddr, "file://", "", 1))
	} else {
		log.Fatalf("invalid blob storage")
	}

	sourceRpmLocation := ""
	if req.ModuleMode {
		sourceRpmLocation = fmt.Sprintf("%s/%s", req.ModulePrefix, req.Package)
	} else {
		sourceRpmLocation = fmt.Sprintf("%s/%s", req.RpmPrefix, req.Package)
	}
	importer = &modes.GitMode{}

	lastKeyLocation := req.SshKeyLocation
	if lastKeyLocation == "" {
		usr, err := user.Current()
		if err != nil {
			log.Fatalf("could not get user: %v", err)
		}
		lastKeyLocation = filepath.Join(usr.HomeDir, ".ssh/id_rsa")
	}

	var authenticator *ssh.PublicKeys

	var err error
	// create ssh key authenticator
	authenticator, err = ssh.NewPublicKeysFromFile(req.SshUser, lastKeyLocation, "")
	if err != nil {
		log.Fatalf("could not get git authenticator: %v", err)
	}

	fsCreator := func(branch string) (billy.Filesystem, error) {
		return memfs.New(), nil
	}
	if req.FsCreator != nil {
		fsCreator = req.FsCreator
	}

	if req.TmpFsMode != "" {
		log.Printf("using tmpfs dir: %s", req.TmpFsMode)
		fsCreator = func(branch string) (billy.Filesystem, error) {
			fs, err := fsCreator(branch)
			if err != nil {
				return nil, err
			}
			tmpDir := filepath.Join(req.TmpFsMode, branch)
			err = fs.MkdirAll(tmpDir, 0755)
			if err != nil {
				log.Fatalf("could not create tmpfs dir: %v", err)
			}
			nFs, err := fs.Chroot(tmpDir)
			if err != nil {
				return nil, err
			}

			return nFs, nil
		}
	}

	var manualCs []string
	if strings.TrimSpace(req.ManualCommits) != "" {
		manualCs = strings.Split(req.ManualCommits, ",")
	}

	return &data.ProcessData{
		Importer:             importer,
		RpmLocation:          sourceRpmLocation,
		UpstreamPrefix:       req.UpstreamPrefix,
		SshKeyLocation:       lastKeyLocation,
		SshUser:              req.SshUser,
		Version:              req.Version,
		BlobStorage:          blobStorage,
		GitCommitterName:     req.GitCommitterName,
		GitCommitterEmail:    req.GitCommitterEmail,
		ModulePrefix:         req.ModulePrefix,
		ImportBranchPrefix:   req.ImportBranchPrefix,
		BranchPrefix:         req.BranchPrefix,
		SingleTag:            req.SingleTag,
		Authenticator:        authenticator,
		NoDupMode:            req.NoDupMode,
		ModuleMode:           req.ModuleMode,
		TmpFsMode:            req.TmpFsMode,
		NoStorageDownload:    req.NoStorageDownload,
		NoStorageUpload:      req.NoStorageUpload,
		ManualCommits:        manualCs,
		ModuleFallbackStream: req.ModuleFallbackStream,
		AllowStreamBranches:  req.AllowStreamBranches,
		FsCreator:            fsCreator,
	}, nil
}

// ProcessRPM checks the RPM specs and discards any remote files
// This functions also sorts files into directories
// .spec files goes into -> SPECS
// metadata files goes to root
// source files goes into -> SOURCES
// all files that are remote goes into .gitignore
// all ignored files' hash goes into .{Name}.metadata
func ProcessRPM(pd *data.ProcessData) error {
	md, err := pd.Importer.RetrieveSource(pd)
	if err != nil {
		return err
	}
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
			return fmt.Errorf("could not init git repo: %v", err)
		}
		remoteUrl := fmt.Sprintf("%s/%s/%s.git", pd.UpstreamPrefix, remotePrefix, gitlabify(md.Name))
		refspec := config.RefSpec("+refs/heads/*:refs/remotes/origin/*")

		remote, err := repo.CreateRemote(&config.RemoteConfig{
			Name:  "origin",
			URLs:  []string{remoteUrl},
			Fetch: []config.RefSpec{refspec},
		})
		if err != nil {
			return fmt.Errorf("could not create remote: %v", err)
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

			head := fmt.Sprintf("refs/tags/imports/%s/%s-%s", branchCommit[0], md.Name, branchCommit[1])
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

		createdFs, err := pd.FsCreator(md.PushBranch)
		if err != nil {
			return err
		}

		// create new Repo for final dist
		repo, err := git.Init(memory.NewStorage(), createdFs)
		if err != nil {
			return fmt.Errorf("could not create new dist Repo: %v", err)
		}
		w, err := repo.Worktree()
		if err != nil {
			return fmt.Errorf("could not get dist Worktree: %v", err)
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
		remoteUrl := fmt.Sprintf("%s/%s/%s.git", pd.UpstreamPrefix, remotePrefix, gitlabify(md.Name))
		log.Printf("using remote: %s", remoteUrl)
		refspec := config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", md.PushBranch, md.PushBranch))
		log.Printf("using refspec: %s", refspec)

		_, err = repo.CreateRemote(&config.RemoteConfig{
			Name:  "origin",
			URLs:  []string{remoteUrl},
			Fetch: []config.RefSpec{refspec},
		})
		if err != nil {
			return fmt.Errorf("could not create remote: %v", err)
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
				return fmt.Errorf("could not set reference: %v", err)
			}
		} else {
			err = w.Checkout(&git.CheckoutOptions{
				Branch: plumbing.NewRemoteReferenceName("origin", md.PushBranch),
				Hash:   hash,
				Force:  true,
			})
			if err != nil {
				return fmt.Errorf("could not checkout: %v", err)
			}
		}

		err = pd.Importer.WriteSource(pd, md)
		if err != nil {
			return err
		}

		data.CopyFromFs(md.Worktree.Filesystem, w.Filesystem, ".")
		md.Repo = repo
		md.Worktree = w

		if pd.ModuleMode {
			err := patchModuleYaml(pd, md)
			if err != nil {
				return err
			}
		} else {
			err := executePatchesRpm(pd, md)
			if err != nil {
				return err
			}
		}

		// get ignored files hash and add to .{Name}.metadata
		metadataFile := fmt.Sprintf(".%s.metadata", md.Name)
		metadata, err := w.Filesystem.Create(metadataFile)
		if err != nil {
			return fmt.Errorf("could not create metadata file: %v", err)
		}
		for _, source := range md.SourcesToIgnore {
			sourcePath := source.Name

			_, err := w.Filesystem.Stat(sourcePath)
			if source.Expired || err != nil {
				continue
			}

			sourceFile, err := w.Filesystem.Open(sourcePath)
			if err != nil {
				return fmt.Errorf("could not open ignored source file %s: %v", sourcePath, err)
			}
			sourceFileBts, err := ioutil.ReadAll(sourceFile)
			if err != nil {
				return fmt.Errorf("could not read the whole of ignored source file: %v", err)
			}

			source.HashFunction.Reset()
			_, err = source.HashFunction.Write(sourceFileBts)
			if err != nil {
				return fmt.Errorf("could not write bytes to hash function: %v", err)
			}
			checksum := hex.EncodeToString(source.HashFunction.Sum(nil))
			checksumLine := fmt.Sprintf("%s %s\n", checksum, sourcePath)
			_, err = metadata.Write([]byte(checksumLine))
			if err != nil {
				return fmt.Errorf("could not write to metadata file: %v", err)
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
			return fmt.Errorf("could not add metadata file: %v", err)
		}

		lastFilesToAdd := []string{".gitignore", "SPECS"}
		for _, f := range lastFilesToAdd {
			_, err := w.Filesystem.Stat(f)
			if err == nil {
				_, err := w.Add(f)
				if err != nil {
					return fmt.Errorf("could not add %s: %v", f, err)
				}
			}
		}

		if pd.TmpFsMode != "" {
			continue
		}

		err = pd.Importer.PostProcess(md)
		if err != nil {
			return err
		}

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
					return fmt.Errorf("could not delete extra file %s: %v", path, err)
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
			return fmt.Errorf("could not commit object: %v", err)
		}

		obj, err := repo.CommitObject(commit)
		if err != nil {
			return fmt.Errorf("could not get commit object: %v", err)
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
			return fmt.Errorf("could not create tag: %v", err)
		}

		pushRefspecs = append(pushRefspecs, config.RefSpec("HEAD:"+plumbing.NewTagReferenceName(newTag)))

		err = repo.Push(&git.PushOptions{
			RemoteName: "origin",
			Auth:       pd.Authenticator,
			RefSpecs:   pushRefspecs,
			Force:      true,
		})
		if err != nil {
			return fmt.Errorf("could not push to remote: %v", err)
		}

		hashString := obj.Hash.String()
		latestHashForBranch[md.PushBranch] = hashString
	}

	err = json.NewEncoder(os.Stdout).Encode(latestHashForBranch)
	if err != nil {
		return fmt.Errorf("could not print hashes")
	}

	return nil
}