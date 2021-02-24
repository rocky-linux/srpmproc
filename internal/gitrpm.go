package internal

import (
	"github.com/cavaliercoder/go-rpm"
)

// TODO: ugly hack, should create an interface
// since GitMode does not parse an RPM file, we just mimick
// the headers of an actual file to reuse RpmFile.Name()
func createPackageFile(name string) *rpm.PackageFile {
	return &rpm.PackageFile{
		Lead: rpm.Lead{},
		Headers: []rpm.Header{
			{},
			{
				Version:    0,
				IndexCount: 1,
				Length:     1,
				Indexes: []rpm.IndexEntry{
					{
						Tag:       1000,
						Type:      rpm.IndexDataTypeStringArray,
						Offset:    0,
						ItemCount: 1,
						Value:     []string{name},
					},
				},
			},
		},
	}
}
