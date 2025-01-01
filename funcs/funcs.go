package funcs

import (
	"github.com/avast/retry-go/v4"
)

func RetriableFunction[In, Out any](funcFactory Function[chan Out, Function[In, error]], errorHandler ...Consumer[error]) Function[In, Out] {
	return func(in In) Out {
		resChan := make(chan Out, 0)
		theFunc := funcFactory(resChan)
		go HandleErrorAsync(func() error { return retry.Do(func() error { return theFunc(in) }) })

		return <-resChan
	}
}

func RetriableSupplier[Out any](funcFactory Function[chan Out, Supplier[error]], errorHandler ...Consumer[error]) Supplier[Out] {
	return func() Out {
		resChan := make(chan Out, 0)
		theFunc := funcFactory(resChan)
		go HandleErrorAsync(func() error { return retry.Do(func() error { return theFunc() }) })

		return <-resChan
	}
}

func HandleError(err error) {
	if err != nil {
		panic(err)
	}
}

func HandleErrorAsync(supplier Supplier[error], errorHandler ...Consumer[error]) {
	theChan := make(chan error)
	go func() {
		theChan <- supplier()
	}()

	err := <-theChan
	if len(errorHandler) > 0 {
		errorHandler[0](err)
	} else {
		HandleError(err)
	}
}
