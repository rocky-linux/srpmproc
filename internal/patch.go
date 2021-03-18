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

func getTipStream(pd *data.ProcessData, module string, pushBranch string, origPushBranch string) string {
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

		log.Println(prefix, ref.Name().String())

		if strings.HasPrefix(ref.Name().String(), prefix) {
			tipHash = ref.Hash().String()
		}
	}

	for _, ref := range list {
		prefix := fmt.Sprintf("refs/heads/%s", origPushBranch)

		if strings.HasPrefix(ref.Name().String(), prefix) {
			tipHash = ref.Hash().String()
		}
	}

	if tipHash == "" {
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
		tipHash = getTipStream(pd, name, pushBranch, md.PushBranch)

		err = module.Marshal(md.Worktree.Filesystem, mdTxtPath)
		if err != nil {
			log.Fatalf("could not marshal modulemd: %v", err)
		}

		rpm.Ref = tipHash
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
