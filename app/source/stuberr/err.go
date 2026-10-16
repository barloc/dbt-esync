package stuberr

import "errors"

// ErrNotImplemented is returned by source stubs until the matching S* task lands.
var ErrNotImplemented = errors.New("not implemented")
