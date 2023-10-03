package rootdir

import (
	"path/filepath"
	"runtime"
)

var (
	_, b, _, _ = runtime.Caller(0)
	Path       = filepath.Join(filepath.Dir(b), "../..")
)
