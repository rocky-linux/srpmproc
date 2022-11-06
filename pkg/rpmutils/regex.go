package rpmutils

import "regexp"

var (
	Nvr = regexp.MustCompile("^(\\S+)-([\\w~%.+]+)-(\\w+(?:\\.[\\w+]+)+?)(?:\\.(\\w+))?(?:\\.rpm)?$")
)
