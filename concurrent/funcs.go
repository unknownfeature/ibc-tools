package concurrent

import (
	"main/funcs"
	"github.com/avast/retry-go"
)

func Go(theFunc func(), errorHandler funcs.Consumer[any]) {
	go func() {
		defer func() {
			if err := recover(); err != nil {
				errorHandler(err)
			}
		}()
		theFunc()
	}
}
