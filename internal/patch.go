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

package internal

import (
	"fmt"
	"git.rockylinux.org/release-engineering/public/srpmproc/internal/data"
	"git.rockylinux.org/release-engineering/public/srpmproc/internal/directives"
	"git.rockylinux.org/release-engineering/public/srpmproc/modulemd"
	srpmprocpb "git.rockylinux.org/release-engineering/public/srpmproc/pb"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	"google.golang.org/protobuf/encoding/prototext"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
	"time"
)

func cfgPatches(pd *data.ProcessData, md *data.ModeData, patchTree *git.Worktree, pushTree *git.Worktree) {
	// check CFG patches
	_, err := patchTree.Filesystem.Stat("ROCKY/CFG")
	if err == nil {
		// iterate through patches
		infos, err := patchTree.Filesystem.ReadDir("ROCKY/CFG")
		if err != nil {
			log.Fatalf("could not walk patches: %v", err)
		}

		for _, info := range infos {
			// can only process .cfg files
			if !strings.HasSuffix(info.Name(), ".cfg") {
				continue
			}

			log.Printf("applying directive %s", info.Name())
			filePath := filepath.Join("ROCKY/CFG", info.Name())
			directive, err := patchTree.Filesystem.Open(filePath)
			if err != nil {
				log.Fatalf("could not open directive file %s: %v", info.Name(), err)
			}
			directiveBytes, err := ioutil.ReadAll(directive)
			if err != nil {
				log.Fatalf("could not read directive file: %v", err)
			}

			var cfg srpmprocpb.Cfg
			err = prototext.Unmarshal(directiveBytes, &cfg)
			if err != nil {
				log.Fatalf("could not unmarshal cfg file: %v", err)
			}

			directives.Apply(&cfg, pd, md, patchTree, pushTree)
		}
	}
}

func applyPatches(pd *data.ProcessData, md *data.ModeData, patchTree *git.Worktree, pushTree *git.Worktree) {
	// check if patches exist
	_, err := patchTree.Filesystem.Stat("ROCKY")
	if err == nil {
		cfgPatches(pd, md, patchTree, pushTree)
	}
}

func executePatchesRpm(pd *data.ProcessData, md *data.ModeData) {
	// fetch patch repository
	repo, err := git.Init(memory.NewStorage(), memfs.New())
	if err != nil {
		log.Fatalf("could not create new dist Repo: %v", err)
	}
	w, err := repo.Worktree()
	if err != nil {
		log.Fatalf("could not get dist Worktree: %v", err)
	}

	remoteUrl := fmt.Sprintf("%s/patch/%s.git", pd.UpstreamPrefix, gitlabify(md.RpmFile.Name()))
	refspec := config.RefSpec(fmt.Sprintf("+refs/heads/*:refs/remotes/origin/*"))

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name:  "origin",
		URLs:  []string{remoteUrl},
		Fetch: []config.RefSpec{refspec},
	})
	if err != nil {
		log.Fatalf("could not create remote: %v", err)
	}

	fetchOptions := &git.FetchOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{refspec},
	}
	if !strings.HasPrefix(pd.UpstreamPrefix, "http") {
		fetchOptions.Auth = pd.Authenticator
	}
	err = repo.Fetch(fetchOptions)

	refName := plumbing.NewBranchReferenceName(md.PushBranch)
	log.Printf("set reference to ref: %s", refName)

	if err != nil {
		// no patches active
		log.Println("info: patch repo not found")
		return
	} else {
		err = w.Checkout(&git.CheckoutOptions{
			Branch: plumbing.NewRemoteReferenceName("origin", "main"),
			Force:  true,
		})
		// common patches found, apply them
		if err == nil {
			applyPatches(pd, md, w, md.Worktree)
		} else {
			log.Println("info: no common patches found")
		}

		err = w.Checkout(&git.CheckoutOptions{
			Branch: plumbing.NewRemoteReferenceName("origin", md.PushBranch),
			Force:  true,
		})
		// branch specific patches found, apply them
		if err == nil {
			applyPatches(pd, md, w, md.Worktree)
		} else {
			log.Println("info: no branch specific patches found")
		}
	}
}

func getTipStream(pd *data.ProcessData, module string, pushBranch string, origPushBranch string, tries int) string {
	repo, err := git.Init(memory.NewStorage(), memfs.New())
	if err != nil {
		log.Fatalf("could not init git Repo: %v", err)
	}

	remoteUrl := fmt.Sprintf("%s/rpms/%s.git", pd.UpstreamPrefix, gitlabify(module))
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
		log.Printf("could not import module: %s", module)
		if tries < 3 {
			log.Printf("could not get rpm refs. will retry in 3s. %v", err)
			time.Sleep(3 * time.Second)
			return getTipStream(pd, module, pushBranch, origPushBranch, tries+1)
		}

		log.Fatalf("could not get rpm refs. import the rpm before the module: %v", err)
	}

	var tipHash string

	for _, ref := range list {
		prefix := fmt.Sprintf("refs/heads/%s", pushBranch)

		branchVersion := strings.Split(pushBranch, "-")
		if len(branchVersion) == 3 {
			version := branchVersion[2]
			// incompatible pattern: v1,v2 etc.
			// example can be found in refs/tags/imports/c8-stream-1.1/subversion-1.10-8030020200519083055.9ce6d490
			if strings.HasPrefix(version, "v") {
				prefix = strings.Replace(prefix, version, strings.TrimPrefix(version, "v"), 1)
			}
		}

		if strings.HasPrefix(ref.Name().String(), prefix) {
			tipHash = ref.Hash().String()
		}
	}

	if tipHash == "" {
		for _, ref := range list {
			prefix := fmt.Sprintf("refs/heads/%s", origPushBranch)

			if strings.HasPrefix(ref.Name().String(), prefix) {
				tipHash = ref.Hash().String()
			}
		}
	}

	if tipHash == "" {
		for _, ref := range list {
			if !strings.Contains(ref.Name().String(), "stream") {
				tipHash = ref.Hash().String()
			}
		}
	}

	if tipHash == "" {
		for _, ref := range list {
			log.Println(pushBranch, ref.Name())
		}
		log.Fatal("could not find tip hash")
	}

	return tipHash
}

func patchModuleYaml(pd *data.ProcessData, md *data.ModeData) {
	// special case for platform.yaml
	_, err := md.Worktree.Filesystem.Open("platform.yaml")
	if err == nil {
		return
	}

	mdTxtPath := "SOURCES/modulemd.src.txt"
	f, err := md.Worktree.Filesystem.Open(mdTxtPath)
	if err != nil {
		log.Fatalf("could not open modulemd file: %v", err)
	}

	content, err := ioutil.ReadAll(f)
	if err != nil {
		log.Fatalf("could not read modulemd file: %v", err)
	}

	module, err := modulemd.Parse(content)
	if err != nil {
		log.Fatalf("could not parse modulemd file: %v", err)
	}

	log.Println("This module contains the following rpms:")
	for name := range module.Data.Components.Rpms {
		log.Printf("\t- %s", name)
	}

	for name, rpm := range module.Data.Components.Rpms {
		var tipHash string
		var pushBranch string
		split := strings.Split(rpm.Ref, "-")
		// TODO: maybe point to correct release tag? but refer to latest for now,
		// we're bootstrapping a new distro for latest RHEL8 anyways. So earlier
		// versions are not that important
		if strings.HasPrefix(rpm.Ref, "stream-rhel-") {
			repString := fmt.Sprintf("%s%ss-", pd.BranchPrefix, string(split[4][0]))
			newString := fmt.Sprintf("%s%s-", pd.BranchPrefix, string(split[4][0]))
			pushBranch = strings.Replace(md.PushBranch, repString, newString, 1)
		} else if strings.HasPrefix(rpm.Ref, "stream-") && len(split) == 2 {
			pushBranch = md.PushBranch
		} else if strings.HasPrefix(rpm.Ref, "stream-") && len(split) == 3 {
			// example: ant
			pushBranch = fmt.Sprintf("%s%d-stream-%s", pd.BranchPrefix, pd.Version, split[2])
		} else if strings.HasPrefix(rpm.Ref, "stream-") {
			pushBranch = fmt.Sprintf("%s%s-stream-%s", pd.BranchPrefix, string(split[3][0]), split[1])
		} else if strings.HasPrefix(rpm.Ref, "rhel-") {
			pushBranch = md.PushBranch
		} else {
			log.Fatal("could not recognize modulemd ref")
		}

		rpm.Ref = pushBranch
		tipHash = getTipStream(pd, name, pushBranch, md.PushBranch, 0)

		err = module.Marshal(md.Worktree.Filesystem, mdTxtPath)
		if err != nil {
			log.Fatalf("could not marshal modulemd: %v", err)
		}

		rpm.Ref = tipHash
	}

	for name, rpm := range module.Data.Components.Rpms {
		if name != gitlabify(name) {
			rpm.Repository = fmt.Sprintf("https://%s/rpms/%s.git", pd.UpstreamPrefixHttps, gitlabify(name))
		}
	}

	rootModule := fmt.Sprintf("%s.yaml", md.RpmFile.Name())
	err = module.Marshal(md.Worktree.Filesystem, rootModule)
	if err != nil {
		log.Fatalf("could not marshal root modulemd: %v", err)
	}

	_, err = md.Worktree.Add(rootModule)
	if err != nil {
		log.Fatalf("could not add root modulemd: %v", err)
	}
}
