package util

import (
	"errors"

	"github.com/vektah/gqlparser/v2/gqlerror"
)

func GetErrorList(err error) gqlerror.List {
	if err == nil {
		return nil
	}

	var list gqlerror.List
	if errors.As(err, &list) {
		return list
	}

	var gerr *gqlerror.Error
	if errors.As(err, &gerr) {
		return append(list, gerr)
	}

	return append(list, gqlerror.WrapPath(nil, err))
}
