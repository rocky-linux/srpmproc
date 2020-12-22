package modulemd

import (
	"github.com/go-git/go-billy/v5"
	"gopkg.in/yaml.v2"
)

type ModuleMd struct {
	Document string `yaml:"document"`
	Version  int    `yaml:"version"`
	Data     struct {
		Name        string `yaml:"name"`
		Stream      string `yaml:"stream"`
		Summary     string `yaml:"summary"`
		Description string `yaml:"description"`
		License     struct {
			Module []string `yaml:"module"`
		} `yaml:"license"`
		Dependencies []struct {
			BuildRequires struct {
				Platform []string `yaml:"platform"`
			} `yaml:"buildrequires"`
			Requires struct {
				Platform []string `yaml:"platform"`
			} `yaml:"requires"`
		} `yaml:"dependencies"`
		References struct {
			Documentation string `yaml:"documentation"`
			Tracker       string `yaml:"tracker"`
		} `yaml:"references"`
		Profiles struct {
			Common struct {
				Rpms []string `yaml:"rpms"`
			} `yaml:"common"`
		} `yaml:"profiles"`
		API struct {
			Rpms []string `yaml:"rpms"`
		} `yaml:"api"`
		Components struct {
			Rpms map[string]*struct {
				Rationale string `yaml:"rationale"`
				Ref       string `yaml:"ref"`
			} `yaml:"rpms"`
		} `yaml:"components"`
	} `yaml:"data"`
}

func Parse(input []byte) (*ModuleMd, error) {
	var ret ModuleMd
	err := yaml.Unmarshal(input, &ret)
	if err != nil {
		return nil, err
	}

	return &ret, nil
}

func (m *ModuleMd) Marshal(fs billy.Filesystem, path string) error {
	bts, err := yaml.Marshal(m)
	if err != nil {
		return err
	}

	_ = fs.Remove(path)
	f, err := fs.Create(path)
	if err != nil {
		return err
	}
	_, err = f.Write(bts)
	if err != nil {
		return err
	}
	_ = f.Close()

	return nil
}
