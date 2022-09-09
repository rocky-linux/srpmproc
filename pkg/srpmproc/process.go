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
	"fmt"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	srpmprocpb "github.com/rocky-linux/srpmproc/pb"
	"github.com/rocky-linux/srpmproc/pkg/blob"
	"github.com/rocky-linux/srpmproc/pkg/blob/file"
	"github.com/rocky-linux/srpmproc/pkg/blob/gcs"
	"github.com/rocky-linux/srpmproc/pkg/blob/s3"
	"github.com/rocky-linux/srpmproc/pkg/misc"
	"github.com/rocky-linux/srpmproc/pkg/modes"
	"github.com/rocky-linux/srpmproc/pkg/rpmutils"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"
	"bufio"

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
	HttpUsername         string
	HttpPassword         string
	ManualCommits        string
	UpstreamPrefix       string
	GitCommitterName     string
	GitCommitterEmail    string
	ImportBranchPrefix   string
	BranchPrefix         string
	FsCreator            data.FsCreatorFunc
	NoDupMode            bool
	BranchSuffix         string
	StrictBranchMode     bool
	ModuleFallbackStream string
	NoStorageUpload      bool
	NoStorageDownload    bool
	SingleTag            string
	CdnUrl               string
	LogWriter            io.Writer

	PackageVersion string
	PackageRelease string

  TaglessMode bool
  AltLookAside bool
}

func gitlabify(str string) string {
	if str == "tree" {
		return "treepkg"
	}

	return strings.Replace(str, "+", "plus", -1)
}

func NewProcessData(req *ProcessDataRequest) (*data.ProcessData, error) {
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
	if req.CdnUrl == "" && req.AltLookAside == false {
		req.CdnUrl = "https://git.centos.org/sources"
	}
	// If altlookaside is enabled, and the CdnUrl hasn't been changed, then automatically set it to the default
	// CentOS Stream (the new pattern very much won't work with the old git.centos.org/sources site)
	if (req.CdnUrl == "https://git.centos.org/sources" || req.CdnUrl == "") && req.AltLookAside == true {
	  req.CdnUrl = "https://sources.stream.centos.org/sources/rpms"
	}

	// Validate required
	if req.Package == "" {
		return nil, fmt.Errorf("package cannot be empty")
	}

	var importer data.ImportMode
	var blobStorage blob.Storage

	if strings.HasPrefix(req.StorageAddr, "gs://") {
		var err error
		blobStorage, err = gcs.New(strings.Replace(req.StorageAddr, "gs://", "", 1))
		if err != nil {
			return nil, err
		}
	} else if strings.HasPrefix(req.StorageAddr, "s3://") {
		blobStorage = s3.New(strings.Replace(req.StorageAddr, "s3://", "", 1))
	} else if strings.HasPrefix(req.StorageAddr, "file://") {
		blobStorage = file.New(strings.Replace(req.StorageAddr, "file://", "", 1))
	} else {
		return nil, fmt.Errorf("invalid blob storage")
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
			return nil, fmt.Errorf("could not get user: %v", err)
		}
		lastKeyLocation = filepath.Join(usr.HomeDir, ".ssh/id_rsa")
	}

	var authenticator transport.AuthMethod

	var err error
	if req.HttpUsername != "" {
		authenticator = &http.BasicAuth{
			Username: req.HttpUsername,
			Password: req.HttpPassword,
		}
	} else {
		// create ssh key authenticator
		authenticator, err = ssh.NewPublicKeysFromFile(req.SshUser, lastKeyLocation, "")
	}
	if err != nil {
		return nil, fmt.Errorf("could not get git authenticator: %v", err)
	}

	fsCreator := func(branch string) (billy.Filesystem, error) {
		if req.TmpFsMode != "" {
			return osfs.New("."), nil
		}
		return memfs.New(), nil
	}
	reqFsCreator := fsCreator
	if req.FsCreator != nil {
		reqFsCreator = req.FsCreator
	}

	var writer io.Writer = os.Stdout
	if req.LogWriter != nil {
		writer = req.LogWriter
	}
	logger := log.New(writer, "", log.LstdFlags)

	if req.TmpFsMode != "" {
		logger.Printf("using tmpfs dir: %s", req.TmpFsMode)
		fsCreator = func(branch string) (billy.Filesystem, error) {
			fs, err := reqFsCreator(branch)
			if err != nil {
				return nil, err
			}
			tmpDir := filepath.Join(req.TmpFsMode, branch)
			err = fs.MkdirAll(tmpDir, 0755)
			if err != nil {
				return nil, fmt.Errorf("could not create tmpfs dir: %v", err)
			}
			nFs, err := fs.Chroot(tmpDir)
			if err != nil {
				return nil, err
			}

			return nFs, nil
		}
	} else {
		fsCreator = reqFsCreator
	}

	var manualCs []string
	if strings.TrimSpace(req.ManualCommits) != "" {
		manualCs = strings.Split(req.ManualCommits, ",")
	}
    
	return &data.ProcessData{
		Importer:             importer,
		RpmLocation:          sourceRpmLocation,
		UpstreamPrefix:       req.UpstreamPrefix,
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
		BranchSuffix:         req.BranchSuffix,
		StrictBranchMode:     req.StrictBranchMode,
		FsCreator:            fsCreator,
		CdnUrl:               req.CdnUrl,
		Log:                  logger,
		PackageVersion:       req.PackageVersion,
		PackageRelease:       req.PackageRelease,
		TaglessMode:          req.TaglessMode,
		AltLookAside:         req.AltLookAside,
	}, nil
}

// ProcessRPM checks the RPM specs and discards any remote files
// This functions also sorts files into directories
// .spec files goes into -> SPECS
// metadata files goes to root
// source files goes into -> SOURCES
// all files that are remote goes into .gitignore
// all ignored files' hash goes into .{Name}.metadata
func ProcessRPM(pd *data.ProcessData) (*srpmprocpb.ProcessResponse, error) {
	
	// if we are using "tagless mode", then we need to jump to a completely different import process:
	// Version info needs to be derived from rpmbuild + spec file, not tags
	if pd.TaglessMode == true {
	  result, err := processRPMTagless(pd)
	  return result, err
	}
	
	md, err := pd.Importer.RetrieveSource(pd)
	if err != nil {
		return nil, err
	}
	md.BlobCache = map[string][]byte{}

	remotePrefix := "rpms"
	if pd.ModuleMode {
		remotePrefix = "modules"
	}

	latestHashForBranch := map[string]string{}
	versionForBranch := map[string]*srpmprocpb.VersionRelease{}

	// already uploaded blobs are skipped
	var alreadyUploadedBlobs []string

	// if no-dup-mode is enabled then skip already imported versions
	var tagIgnoreList []string
	if pd.NoDupMode {
		repo, err := git.Init(memory.NewStorage(), memfs.New())
		if err != nil {
			return nil, fmt.Errorf("could not init git repo: %v", err)
		}
		remoteUrl := fmt.Sprintf("%s/%s/%s.git", pd.UpstreamPrefix, remotePrefix, gitlabify(md.Name))
		refspec := config.RefSpec("+refs/heads/*:refs/remotes/origin/*")

		remote, err := repo.CreateRemote(&config.RemoteConfig{
			Name:  "origin",
			URLs:  []string{remoteUrl},
			Fetch: []config.RefSpec{refspec},
		})
		if err != nil {
			return nil, fmt.Errorf("could not create remote: %v", err)
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
				return nil, fmt.Errorf("invalid manual commit list")
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

		var matchString string
		if !misc.GetTagImportRegex(pd).MatchString(md.TagBranch) {
			if pd.ModuleMode {
				prefix := fmt.Sprintf("refs/heads/%s%d", pd.ImportBranchPrefix, pd.Version)
				if strings.HasPrefix(md.TagBranch, prefix) {
					replace := strings.Replace(md.TagBranch, "refs/heads/", "", 1)
					matchString = fmt.Sprintf("refs/tags/imports/%s/%s", replace, filepath.Base(pd.RpmLocation))
					pd.Log.Printf("using match string: %s", matchString)
				}
			}
			if !misc.GetTagImportRegex(pd).MatchString(matchString) {
				continue
			}
		} else {
			  matchString = md.TagBranch
		}

    
		match := misc.GetTagImportRegex(pd).FindStringSubmatch(matchString)
				
		md.PushBranch = pd.BranchPrefix + strings.TrimPrefix(match[2], pd.ImportBranchPrefix)
		
		newTag := "imports/" + pd.BranchPrefix + strings.TrimPrefix(match[1], "imports/"+pd.ImportBranchPrefix)
		newTag = strings.Replace(newTag, "%", "_", -1)

		createdFs, err := pd.FsCreator(md.PushBranch)
		if err != nil {
      return nil, err
		}

		// create new Repo for final dist
		repo, err := git.Init(memory.NewStorage(), createdFs)
		if err != nil {
			return nil, fmt.Errorf("could not create new dist Repo: %v", err)
		}
		w, err := repo.Worktree()
		if err != nil {
			return nil, fmt.Errorf("could not get dist Worktree: %v", err)
		}

		shouldContinue := true
		for _, ignoredTag := range tagIgnoreList {
			if ignoredTag == "refs/tags/"+newTag {
				pd.Log.Printf("skipping %s", ignoredTag)
				shouldContinue = false
			}
		}
		if !shouldContinue {
			continue
		}

		// create a new remote
		remoteUrl := fmt.Sprintf("%s/%s/%s.git", pd.UpstreamPrefix, remotePrefix, gitlabify(md.Name))
		pd.Log.Printf("using remote: %s", remoteUrl)
		refspec := config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", md.PushBranch, md.PushBranch))
		pd.Log.Printf("using refspec: %s", refspec)

		_, err = repo.CreateRemote(&config.RemoteConfig{
			Name:  "origin",
			URLs:  []string{remoteUrl},
			Fetch: []config.RefSpec{refspec},
		})
		if err != nil {
			return nil, fmt.Errorf("could not create remote: %v", err)
		}

		err = repo.Fetch(&git.FetchOptions{
			RemoteName: "origin",
			RefSpecs:   []config.RefSpec{refspec},
			Auth:       pd.Authenticator,
		})

		refName := plumbing.NewBranchReferenceName(md.PushBranch)
		pd.Log.Printf("set reference to ref: %s", refName)

		var hash plumbing.Hash
		if commitPin[md.PushBranch] != "" {
			hash = plumbing.NewHash(commitPin[md.PushBranch])
		}

		if err != nil {
			h := plumbing.NewSymbolicReference(plumbing.HEAD, refName)
			if err := repo.Storer.CheckAndSetReference(h, nil); err != nil {
				return nil, fmt.Errorf("could not set reference: %v", err)
			}
		} else {
			err = w.Checkout(&git.CheckoutOptions{
				Branch: plumbing.NewRemoteReferenceName("origin", md.PushBranch),
				Hash:   hash,
				Force:  true,
			})
			if err != nil {
				return nil, fmt.Errorf("could not checkout: %v", err)
			}
		}

		err = pd.Importer.WriteSource(pd, md)
		if err != nil {
			return nil, err
		}

		err = data.CopyFromFs(md.Worktree.Filesystem, w.Filesystem, ".")
		if err != nil {
			return nil, err
		}
		md.Repo = repo
		md.Worktree = w

		if pd.ModuleMode {
			err := patchModuleYaml(pd, md)
			if err != nil {
				return nil, err
			}
		} else {
			err := executePatchesRpm(pd, md)
			if err != nil {
				return nil, err
			}
		}

		// get ignored files hash and add to .{Name}.metadata
    metadataFile := ""
		ls, err := md.Worktree.Filesystem.ReadDir(".")
		if err != nil {
			return nil, fmt.Errorf("could not read directory: %v", err)
		}
		for _, f := range ls {
			if strings.HasSuffix(f.Name(), ".metadata") {
				if metadataFile != "" {
					return nil, fmt.Errorf("multiple metadata files found")
				}
				metadataFile = f.Name()
			}
		}
		if metadataFile == "" {
			metadataFile = fmt.Sprintf(".%s.metadata", md.Name)
		}
		metadata, err := w.Filesystem.Create(metadataFile)
		if err != nil {
			return nil, fmt.Errorf("could not create metadata file: %v", err)
		}
		for _, source := range md.SourcesToIgnore {
			sourcePath := source.Name

			_, err := w.Filesystem.Stat(sourcePath)
			if source.Expired || err != nil {
				continue
			}

			sourceFile, err := w.Filesystem.Open(sourcePath)
			if err != nil {
				return nil, fmt.Errorf("could not open ignored source file %s: %v", sourcePath, err)
			}
			sourceFileBts, err := ioutil.ReadAll(sourceFile)
			if err != nil {
				return nil, fmt.Errorf("could not read the whole of ignored source file: %v", err)
			}

			source.HashFunction.Reset()
			_, err = source.HashFunction.Write(sourceFileBts)
			if err != nil {
				return nil, fmt.Errorf("could not write bytes to hash function: %v", err)
			}
			checksum := hex.EncodeToString(source.HashFunction.Sum(nil))
			checksumLine := fmt.Sprintf("%s %s\n", checksum, sourcePath)
			_, err = metadata.Write([]byte(checksumLine))
			if err != nil {
				return nil, fmt.Errorf("could not write to metadata file: %v", err)
			}

			if data.StrContains(alreadyUploadedBlobs, checksum) {
				continue
			}
			exists, err := pd.BlobStorage.Exists(checksum)
			if err != nil {
				return nil, err
			}
			if !exists && !pd.NoStorageUpload {
				err := pd.BlobStorage.Write(checksum, sourceFileBts)
				if err != nil {
					return nil, err
				}
				pd.Log.Printf("wrote %s to blob storage", checksum)
			}
			alreadyUploadedBlobs = append(alreadyUploadedBlobs, checksum)
		}

		_, err = w.Add(metadataFile)
		if err != nil {
			return nil, fmt.Errorf("could not add metadata file: %v", err)
		}

		lastFilesToAdd := []string{".gitignore", "SPECS"}
		for _, f := range lastFilesToAdd {
			_, err := w.Filesystem.Stat(f)
			if err == nil {
				_, err := w.Add(f)
				if err != nil {
					return nil, fmt.Errorf("could not add %s: %v", f, err)
				}
			}
		}

		nvrMatch := rpmutils.Nvr.FindStringSubmatch(match[3])
		if len(nvrMatch) >= 4 {
			versionForBranch[md.PushBranch] = &srpmprocpb.VersionRelease{
				Version: nvrMatch[2],
				Release: nvrMatch[3],
			}
		}

		if pd.TmpFsMode != "" {
			continue
		}

		err = pd.Importer.PostProcess(md)
		if err != nil {
			return nil, err
		}

		// show status
		status, _ := w.Status()
		pd.Log.Printf("successfully processed:\n%s", status)

		statusLines := strings.Split(status.String(), "\n")
		for _, line := range statusLines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "D") {
				path := strings.TrimPrefix(trimmed, "D ")
				_, err := w.Remove(path)
				if err != nil {
					return nil, fmt.Errorf("could not delete extra file %s: %v", path, err)
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
			pd.Log.Printf("tip %s", head.String())
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
			return nil, fmt.Errorf("could not commit object: %v", err)
		}

		obj, err := repo.CommitObject(commit)
		if err != nil {
			return nil, fmt.Errorf("could not get commit object: %v", err)
		}

		pd.Log.Printf("committed:\n%s", obj.String())

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
			return nil, fmt.Errorf("could not create tag: %v", err)
		}

		pushRefspecs = append(pushRefspecs, config.RefSpec("HEAD:"+plumbing.NewTagReferenceName(newTag)))
		

		err = repo.Push(&git.PushOptions{
			RemoteName: "origin",
			Auth:       pd.Authenticator,
			RefSpecs:   pushRefspecs,
			Force:      true,
		})
		if err != nil {
			return nil, fmt.Errorf("could not push to remote: %v", err)
		}

		hashString := obj.Hash.String()
		latestHashForBranch[md.PushBranch] = hashString
	}
	
	return &srpmprocpb.ProcessResponse{
		BranchCommits:  latestHashForBranch,
		BranchVersions: versionForBranch,
	}, nil
}


// Process for when we want to import a tagless repo (like from CentOS Stream)
//
func processRPMTagless(pd *data.ProcessData) (*srpmprocpb.ProcessResponse, error) {
  pd.Log.Println("Tagless mode detected, attempting import of latest commit")
  
  // In tagless mode, we *automatically* set StrictBranchMode to true
  // Only the exact <PREFIX><VERSION><SUFFIX> branch should be pulled from the source repo
  pd.StrictBranchMode = true
  
  // our return values: a mapping of branches -> commits (1:1) that we're bringing in, 
  // and a mapping of branches to: version = X, release = Y 
  latestHashForBranch := map[string]string{}
	versionForBranch := map[string]*srpmprocpb.VersionRelease{}
  
  
  md, err := pd.Importer.RetrieveSource(pd)
  if err != nil {
    pd.Log.Println("Error detected in  RetrieveSource!")
    return nil, err
  }
  
  md.BlobCache = map[string][]byte{}

  // TODO: add tagless module support  
  remotePrefix := "rpms"
  if pd.ModuleMode {
    remotePrefix = "modules"
  }
    

  // Set up our remote URL for pushing our repo to
	var tagIgnoreList []string
	if pd.NoDupMode {
		repo, err := git.Init(memory.NewStorage(), memfs.New())
		if err != nil {
			return nil, fmt.Errorf("could not init git repo: %v", err)
		}
		remoteUrl := fmt.Sprintf("%s/%s/%s.git", pd.UpstreamPrefix, remotePrefix, gitlabify(md.Name))
		refspec := config.RefSpec("+refs/heads/*:refs/remotes/origin/*")

		remote, err := repo.CreateRemote(&config.RemoteConfig{
			Name:  "origin",
			URLs:  []string{remoteUrl},
			Fetch: []config.RefSpec{refspec},
		})
		if err != nil {
			return nil, fmt.Errorf("could not create remote: %v", err)
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
  localPath := ""

  for _, branch := range md.Branches {
    md.Repo = &sourceRepo
		md.Worktree = &sourceWorktree
		md.TagBranch = branch
		
		for _, source := range md.SourcesToIgnore {
			source.Expired = true
		}
  
    // Create a temporary place to check out our tag/branch : /tmp/srpmproctmp_<PKG_NAME><RANDOMSTRING>/
    localPath, _ = os.MkdirTemp("/tmp", fmt.Sprintf("srpmproctmp_%s", md.Name))

    if err := os.RemoveAll(localPath); err != nil {
      return nil, fmt.Errorf("Could not remove previous temporary directory: %s", localPath)
    }
    if err := os.Mkdir(localPath, 0755); err != nil {
      return nil, fmt.Errorf("Could not create temporary directory: %s", localPath)
    }
    
    // Clone repo into the temporary path, but only the tag we're interested in:
    // (TODO: will probably need to assign this a variable or use the md struct gitrepo object to perform a successful tag+push later)
    _, _ = git.PlainClone(localPath, false, &git.CloneOptions{
  		URL: pd.RpmLocation,
  		SingleBranch: true,
  		ReferenceName: plumbing.ReferenceName(branch),
  	})

    // Now that we're cloned into localPath, we need to "covert" the import into the old format
    // We want sources to become .PKGNAME.metadata, we want SOURCES and SPECS folders, etc.
    repoFixed, _ := convertLocalRepo(md.Name, localPath)
    if !repoFixed {
      return nil, fmt.Errorf("Error converting repository into SOURCES + SPECS + .package.metadata format")
    }  
  
    
    nvrString := getVersionFromSpec(md.Name, localPath, pd.Version)
    if nvrString == "" {
      return nil, fmt.Errorf("Error using rpm or rpmbuild to build SRPM and determine version info! (tagless mode)")
    }
    
    // Set version and release fields we extracted (name|version|release are separated by pipes)
    pd.PackageVersion = strings.Split(nvrString, "|")[1]
    pd.PackageRelease = strings.Split(nvrString, "|")[2]
    
    
    // Set full rpm version:  name-version-release (for tagging properly)
    rpmVersion := fmt.Sprintf("%s-%s-%s", md.Name, pd.PackageVersion, pd.PackageRelease)
    
    
    pd.Log.Println("Successfully determined version of tagless checkout: ", rpmVersion)
  
    md.PushBranch = fmt.Sprintf("%s%d%s", pd.BranchPrefix, pd.Version, pd.BranchSuffix)
    
            
    // Make an initial repo we will use to push to our target
    pushRepo, err := git.PlainInit(localPath + "_gitpush", false)
 		if err != nil {
			return nil, fmt.Errorf("could not create new dist Repo: %v", err)
		}

    w, err := pushRepo.Worktree()
		if err != nil {
			return nil, fmt.Errorf("could not get dist Worktree: %v", err)
		}
        
    
    // Create a remote "origin" in our empty git, make the upstream equal to the branch we want to modify
    pushUrl := fmt.Sprintf("%s/%s/%s.git", pd.UpstreamPrefix, remotePrefix, gitlabify(md.Name))
    refspec := config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", md.PushBranch, md.PushBranch))
    
    // Make our remote repo the target one - the one we want to push our update to
    pushRepoRemote, err := pushRepo.CreateRemote(&config.RemoteConfig{
			Name:  "origin",
			URLs:  []string{pushUrl},
			Fetch: []config.RefSpec{refspec},
		})
 		if err != nil {
			return nil, fmt.Errorf("could not create remote: %v", err)
		}
    
    // fetch our branch data (md.PushBranch) into this new repo
    err = pushRepo.Fetch(&git.FetchOptions{
			RemoteName: "origin",
			RefSpecs:   []config.RefSpec{refspec},
			Auth:       pd.Authenticator,
		})

    
    refName := plumbing.NewBranchReferenceName(md.PushBranch)

		var hash plumbing.Hash
		h := plumbing.NewSymbolicReference(plumbing.HEAD, refName)
		if err := pushRepo.Storer.CheckAndSetReference(h, nil); err != nil {
				return nil, fmt.Errorf("Could not set symbolic reference: %v", err)
		}
		
		err = w.Checkout(&git.CheckoutOptions{
		  Branch: plumbing.NewRemoteReferenceName("origin", md.PushBranch),
		  Hash: hash,
		  Force: true,
		})
		
    
    os.Rename(fmt.Sprintf("%s/SPECS", localPath),  fmt.Sprintf("%s_gitpush/SPECS", localPath) )
    os.Rename(fmt.Sprintf("%s/SOURCES", localPath),  fmt.Sprintf("%s_gitpush/SOURCES", localPath) )
    os.Rename(fmt.Sprintf("%s/.gitignore", localPath),  fmt.Sprintf("%s_gitpush/.gitignore", localPath) )
    os.Rename(fmt.Sprintf("%s/.%s.metadata", localPath, md.Name ),  fmt.Sprintf("%s_gitpush/.%s.metadata", localPath, md.Name) )
    
    
    
    md.Repo = pushRepo
    md.Worktree = w

    // Download lookaside sources (tarballs) into the push git repo:
    err = pd.Importer.WriteSource(pd, md)
		if err != nil {
			return nil, err
		}
    
    
    // Call function to upload source to target lookaside and 
    // ensure the sources are added to .gitignore
    
    err = processLookasideSources(pd, md, localPath + "_gitpush")
    if err != nil {
      return nil, err
    }
    
    
    // Apply patch(es) if needed:
    if pd.ModuleMode {
			err := patchModuleYaml(pd, md)
			if err != nil {
				return nil, err
			}
		} else {
			err := executePatchesRpm(pd, md)
			if err != nil {
				return nil, err
			}
		}
    
    
    
    err = w.AddWithOptions(&git.AddOptions{All: true} )
    if err != nil {
      return nil, fmt.Errorf("Error adding SOURCES/ , SPECS/ or .metadata file to commit list.")
    }
    
    status, err := w.Status()
    fmt.Println("Current repo status == ", status)
    
    
    // assign tag for our new remote we're about to push (derived from the SRPM version)
    newTag := "refs/tags/imports/" + md.PushBranch + "/" + rpmVersion
    newTag = strings.Replace(newTag, "%", "_", -1)

    // pushRefspecs is a list of all the references we want to push (tags + heads)
    // It's an array of colon-separated strings which map local references to their remote counterparts
    var pushRefspecs []config.RefSpec
    
    
    // We need to find out if the remote repo already has this branch
    // If it doesn't, we want to add *:* to our references for commit.  This will allow us to push the new branch
    // If it does, we can simply push HEAD:refs/heads/<BRANCH>
    newRepo := true
    refList, _ := pushRepoRemote.List(&git.ListOptions{Auth: pd.Authenticator,})
    for _, ref := range refList {
      if strings.HasSuffix(ref.Name().String(),  fmt.Sprintf("heads/%s", md.PushBranch)) {
        newRepo = false
        break
      }
    }

    if newRepo == true {
      pushRefspecs = append(pushRefspecs, config.RefSpec("*:*"))
      pd.Log.Printf("New remote repo detected, creating new remote branch")
    }
    
    
    
    // Identify specific references we want to push
    // Should be refs/heads/<target_branch>, and a tag called imports/<target_branch>/<rpm_nvr>    
    pushRefspecs = append(pushRefspecs, config.RefSpec(fmt.Sprintf("HEAD:refs/heads/%s", md.PushBranch)) )
    pushRefspecs = append(pushRefspecs, config.RefSpec(fmt.Sprintf("HEAD:%s", newTag)) )

    
    // Actually do the commit (locally)
    commit, err := w.Commit("import from tagless source "+pd.Importer.ImportName(pd, md), &git.CommitOptions{
			Author: &object.Signature{
				Name:  pd.GitCommitterName,
				Email: pd.GitCommitterEmail,
				When:  time.Now(),
			},
		})        
    if err != nil {
			return nil, fmt.Errorf("could not commit object: %v", err)
		}

 		obj, err := pushRepo.CommitObject(commit)
		if err != nil {
			return nil, fmt.Errorf("could not get commit object: %v", err)
		}

		pd.Log.Printf("Committed local repo tagless mode transform:\n%s", obj.String())
    
       
    
    // After commit, we will now tag our local repo on disk:
    _, err = pushRepo.CreateTag(newTag, commit, &git.CreateTagOptions{
			Tagger: &object.Signature{
				Name:  pd.GitCommitterName,
				Email: pd.GitCommitterEmail,
				When:  time.Now(),
			},
			Message: "import " + md.TagBranch + " from " + pd.RpmLocation + "(import from tagless source)",
			SignKey: nil,
		})
    if err != nil {
			return nil, fmt.Errorf("could not create tag: %v", err)
		}
    
    pd.Log.Printf("Pushing these references to the remote:  %+v \n", pushRefspecs)


    // Do the actual push to the remote target repository
    err = pushRepo.Push(&git.PushOptions{
      RemoteName: "origin",
      Auth: pd.Authenticator,
      RefSpecs: pushRefspecs,
      Force: true,
    })
    
    if err != nil {
			return nil, fmt.Errorf("could not push to remote: %v", err)
		}

    if err := os.RemoveAll(localPath); err != nil {
      log.Printf("Error cleaning up temporary git checkout directory %s .  Non-fatal, continuing anyway...\n", localPath)
    }
    if err := os.RemoveAll(fmt.Sprintf("%s_gitpush", localPath)); err != nil {
      log.Printf("Error cleaning up temporary git checkout directory %s .  Non-fatal, continuing anyway...\n", fmt.Sprintf("%s_gitpush", localPath))
    }
    
    // append our processed branch to the return structures:
    latestHashForBranch[md.PushBranch] = obj.Hash.String()
    
    versionForBranch[md.PushBranch] = &srpmprocpb.VersionRelease{
      Version: pd.PackageVersion,
      Release: pd.PackageRelease,
    }
    
  }
  
  fmt.Printf("returning::\n latestHashForBranch == %+v \n\n  versionForBranch == %+v\n\n", latestHashForBranch, versionForBranch)
  
  // return struct with all our branch:commit and branch:version+release mappings
  return &srpmprocpb.ProcessResponse{
    BranchCommits:  latestHashForBranch,
    BranchVersions: versionForBranch,
  }, nil
   
}


// Given a local repo on disk, ensure it's in the "traditional" format.  This means:
//   - metadata file is named .pkgname.metadata
//   - metadata file has the old "<SHASUM>  SOURCES/<filename>"  format
//   - SPECS/ and SOURCES/ exist and are populated correctly
func convertLocalRepo(pkgName string, localRepo string)   (bool, error)  {
  
  // Make sure we have a SPECS and SOURCES folder made:
  if err := os.MkdirAll(fmt.Sprintf("%s/SOURCES", localRepo), 0755); err != nil {
      return false, fmt.Errorf("Could not create SOURCES directory in: %s", localRepo)
  }
  
  if err := os.MkdirAll(fmt.Sprintf("%s/SPECS", localRepo), 0755); err != nil {
      return false, fmt.Errorf("Could not create SPECS directory in: %s", localRepo)
  }
    
  // Loop through each file/folder and operate accordingly:
  files, err := ioutil.ReadDir(localRepo)
  if err != nil {
        return false, err
  }
  
  for _, file := range files {
    
    // We don't want to process SOURCES, SPECS, or any of our .git folders
    if file.Name() == "SOURCES" || file.Name() == "SPECS" || strings.HasPrefix(file.Name(), ".git") || file.Name() == "." + pkgName + ".metadata" {
      continue
    }
      
    // If we have a metadata "sources" file, we need to read it and convert to the old .<pkgname>.metadata format
    if file.Name() == "sources" {
      convertStatus := convertMetaData(pkgName, localRepo)
      
      if convertStatus != true {
        return false, fmt.Errorf("Error converting sources metadata file to .metadata format")
      }
      
      continue
    }
    
    // Any file that ends in a ".spec" should be put into SPECS/
    if strings.HasSuffix(file.Name(), ".spec") {
      err := os.Rename(fmt.Sprintf("%s/%s", localRepo, file.Name()),  fmt.Sprintf("%s/SPECS/%s", localRepo, file.Name()) )
      if err != nil {
        return false, fmt.Errorf("Error moving .spec file to SPECS/")
      }
    }
    
    // if a file isn't skipped in one of the above checks, then it must be a file that belongs in SOURCES/
    os.Rename(fmt.Sprintf("%s/%s", localRepo, file.Name()),  fmt.Sprintf("%s/SOURCES/%s", localRepo, file.Name()) )
  }
  
  return true, nil
}


// Given a local "sources" metadata file (new CentOS Stream format), convert it into the older 
// classic CentOS style:  "<HASH>  SOURCES/<FILENAME>"
func convertMetaData(pkgName string, localRepo string) (bool) {
    
    lookAside, err := os.Open(fmt.Sprintf("%s/sources", localRepo))
    if err != nil {
      return false
    }
      
    // Split file into lines and start processing:
    scanner := bufio.NewScanner(lookAside)
    scanner.Split(bufio.ScanLines)
    
    // convertedLA is our array of new "converted" lookaside lines
    var convertedLA []string
    
    // loop through each line, and:
    //   - split by whitespace
    //   - check each line begins with "SHA" or "MD" - validate
    //   - take the 
    // Then check
    for scanner.Scan() {
      
      tmpLine := strings.Fields(scanner.Text())
      // make sure line starts with a "SHA" or "MD" before processing - otherwise it might not be a valid format lookaside line!
      if !(strings.HasPrefix(tmpLine[0], "SHA") || strings.HasPrefix(tmpLine[0], "MD")) {
        continue
      }
      
      // Strip out "( )" characters from file name and prepend SOURCES/ to it
      tmpLine[1] = strings.ReplaceAll(tmpLine[1], "(", "" )
      tmpLine[1] = strings.ReplaceAll(tmpLine[1], ")", "" )
      tmpLine[1] = fmt.Sprintf("SOURCES/%s", tmpLine[1])
      
      convertedLA = append(convertedLA, fmt.Sprintf("%s %s", tmpLine[3], tmpLine[1]) )
      
    }
    lookAside.Close()
    
    // open .<NAME>.metadata file for writing our old-format lines 
    lookAside, err = os.OpenFile(fmt.Sprintf("%s/.%s.metadata", localRepo, pkgName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
      fmt.Errorf("Error opening new .metadata file for writing.")
      return false
    }
    
    writer := bufio.NewWriter(lookAside)
    
    for _, convertedLine := range convertedLA {
      _, _ = writer.WriteString(convertedLine + "\n")
    } 
    
    writer.Flush()
    lookAside.Close()
    
    // Remove old "sources" metadata file - we don't need it now that conversion is complete
    os.Remove(fmt.Sprintf("%s/sources", localRepo))
    
    return true
}


// Given a local checked out folder and package name, including SPECS/ , SOURCES/ , and .package.metadata, this will:
//   - create a "dummy" SRPM (using dummy sources files we use to populate tarballs from lookaside)
//   - extract RPM version info from that SRPM, and return it
// If we are in tagless mode, we need to get a package version somehow!
func getVersionFromSpec(pkgName string, localRepo string, majorVersion int) (string) {

  // Make sure we have "rpm" and "rpmbuild" and "cp" available in our PATH.  Otherwise, this won't work:
  _, err := exec.LookPath("rpm")
	if err != nil {
		return ""
	}

  _, err = exec.LookPath("rpmbuild")
	if err != nil {
		return ""
	}
	
	_, err = exec.LookPath("cp")
	if err != nil {
		return ""
	}
	

  // create separate temp folder space to do our RPM work - we don't want to accidentally contaminate the main Git area:
  rpmBuildPath := fmt.Sprintf("%s_rpm", localRepo)
  os.Mkdir(rpmBuildPath, 0755 )
  
  // Copy SOURCES/ and SPECS/ into the temp rpmbuild directory recursively
  // Yes, we could create or import an elaborate Go-native way to do this, but damnit this is easier:
  cmdArgs := strings.Fields(fmt.Sprintf("cp -rp %s/SOURCES %s/SPECS %s/",  localRepo, localRepo, rpmBuildPath))
  if err := exec.Command(cmdArgs[0], cmdArgs[1:]... ).Run(); err != nil {
	  log.Println(err)
	  return ""
  }

  
  // Loop through .<package>.metadata and get the file names we need to make our SRPM:  
  lookAside, err := os.Open(fmt.Sprintf("%s/.%s.metadata", localRepo, pkgName) )
  if err != nil {
    log.Println(err)
    return ""
  }
      
  // Split file into lines and start processing:
  scanner := bufio.NewScanner(lookAside)
  scanner.Split(bufio.ScanLines)
  
  // loop through each line, and:
  //   - isolate the SOURCES/filename entry
  //   - write out a dummy file of the same name to rpmBuildPath/SOURCES
  for scanner.Scan() {
    
    // lookaside source is always the 2nd part of the line (after the long SHA sum)
    srcFile := strings.Fields(scanner.Text())[1]
    
    // write a dummy file of the same name into the rpmbuild SOURCES/ directory:
    dummyFile, err := os.OpenFile(fmt.Sprintf("%s/%s", rpmBuildPath, srcFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
      return ""
    }
    writer := bufio.NewWriter(dummyFile)
    _, _ = writer.WriteString("This is a dummy lookaside file generated by srpmproc.  It is only needed to get a working SRPM and extract version information.  Please disregard\n")
    writer.Flush()
    dummyFile.Close()
  }
  
  lookAside.Close()


  // Now, call rpmbuild to produce the dummy src file:
  // Example:  rpmbuild  --define "_topdir  /tmp/srpmproctmp_httpd1988142783_rpm"  -bs /tmp/srpmproctmp_httpd1988142783_rpm/SPECS/*.spec
  cmd := exec.Command("rpmbuild", fmt.Sprintf(`--define=_topdir  %s`, rpmBuildPath), fmt.Sprintf(`--define=dist  .el%d`, majorVersion), "-bs", fmt.Sprintf("%s/SPECS/%s.spec", rpmBuildPath, pkgName) )
  if err := cmd.Run(); err != nil {
	  log.Println(err)
	  return ""
  }
  
  // Read the first file from the SRPMS/ folder in rpmBuildPath.  It should be the SRPM that rpmbuild produced above
  // (there should only be one file - we check that it ends in ".rpm" just to be sure!)
  lsTmp, err := ioutil.ReadDir(fmt.Sprintf("%s/SRPMS/", rpmBuildPath))
  if err != nil {
    log.Println(err)
    return ""
  }
  
  srpmFile := lsTmp[0].Name()
  
  if !strings.HasSuffix(srpmFile, ".rpm") {
    log.Println("Error, file found in dummy SRPMS directory did not have an .rpm extension!  Perhaps rpmbuild didn't produce a proper source RPM?")
    return ""
  }
  
  
  // Call the rpm binary to extract the version-release info out of it, and tack on ".el<VERSION>" at the end:
  cmd = exec.Command("rpm", "-qp", "--qf", `%{NAME}|%{VERSION}|%{RELEASE}\n`, fmt.Sprintf("%s/SRPMS/%s", rpmBuildPath, srpmFile) )
    nvrTmp, err := cmd.CombinedOutput()
  if err != nil {
    log.Println("Error running rpm command to extract temporary SRPM name-version-release identifiers.")
    log.Println("rpmbuild output: ", string(nvrTmp))
    log.Println("rpmbuild command: ", cmd.String())
    return ""
  }

  // Pull first line of the rpm command's output to get the name-version-release number (there should only be 1 line)
  nvr := string(nvrTmp)  
  nvr = strings.Fields(nvr)[0]
    
  // Clean up: delete the temporary directory
  if err := os.RemoveAll(rpmBuildPath); err != nil {
    log.Printf("Error cleaning up temporary RPM directory %s .  Non-fatal, continuing anyway...\n", rpmBuildPath)
  }

  // return name-version-release string we derived:
  log.Printf("Derived NVR %s from tagless repo via temporary SRPM build\n", nvr)
  return nvr
  
}



// We need to loop through the lookaside blob files ("SourcesToIgnore"), 
// and upload them to our target storage (usually an S3 bucket, but could be a local folder)
// 
// We also need to add the source paths to .gitignore in the git repo, so we don't accidentally commit + push them
func processLookasideSources(pd *data.ProcessData, md *data.ModeData, localDir string) (error) {
 	
 	w := md.Worktree
 	metadata, err := w.Filesystem.Create(fmt.Sprintf(".%s.metadata", md.Name) )
	if err != nil {
	  return fmt.Errorf("could not create metadata file: %v", err)
	}
  
  // Keep track of files we've already uploaded - don't want duplicates!  
  var alreadyUploadedBlobs []string

  
  gitIgnore, err := os.OpenFile(fmt.Sprintf("%s/.gitignore", localDir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
  if err != nil {
    return err
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
		exists, err := pd.BlobStorage.Exists(checksum)
		if err != nil {
			return  err
		}
		if !exists && !pd.NoStorageUpload {
			err := pd.BlobStorage.Write(checksum, sourceFileBts)
			if err != nil {
				return err
			}
			pd.Log.Printf("wrote %s to blob storage", checksum)
		}
		alreadyUploadedBlobs = append(alreadyUploadedBlobs, checksum) 
	
	  // Add this SOURCES/ lookaside file to be excluded
	  w.Excludes = append(w.Excludes, gitignore.ParsePattern(sourcePath, nil) ) 
	
	  // Append the SOURCES/<file> path to .gitignore:
	  _, err = gitIgnore.Write([]byte(fmt.Sprintf("%s\n", sourcePath)) )
	  if err != nil {
	    return err
	  }
	
	}

  	
	err = gitIgnore.Close()
	if err != nil {
	  return err
	}
	
  return nil

}






