package funcs

import (
	"github.com/avast/retry-go/v4"
	"main/utils"
)

func RetryCallAndReturn[In, Out any](funcFactory Function[chan Out, Function[In, error]]) Function[In, Out] {
	return func(in In) Out {
		resChan := make(chan Out, 0)
		theFunc := funcFactory(resChan)
		utils.HandleError(retry.Do(func() error { return theFunc(in) }))

		return <-resChan
	}
}

func RetrySupplyAndReturn[Out any](funcFactory Function[chan Out, Supplier[error]]) Supplier[Out] {
	return func() Out {
		resChan := make(chan Out, 0)
		theFunc := funcFactory(resChan)
		utils.HandleError(retry.Do(func() error { return theFunc() }))

		return <-resChan
	}
}
