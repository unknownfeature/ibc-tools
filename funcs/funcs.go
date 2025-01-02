package funcs

import (
	"time"
)

func RetriableFunction[In, Out any](funcFactory Function[chan Out, Function[In, error]], errorHandler ...Consumer[error]) Function[In, Out] {
	return func(in In) Out {
		resChan := make(chan Out, 0)
		theFunc := funcFactory(resChan)
		go HandleErrorAsync(func() error { return Retry(func() error { return theFunc(in) }) })

		return <-resChan
	}
}

func RetriableSupplier[Out any](funcFactory Function[chan Out, Supplier[error]], errorHandler ...Consumer[error]) Supplier[Out] {
	return func() Out {
		resChan := make(chan Out, 0)
		theFunc := funcFactory(resChan)
		go HandleErrorAsync(func() error { return Retry(func() error { return theFunc() }) }, errorHandler...)

		return <-resChan
	}
}

func RetriableSupplierWithConfig[Out any](funcFactory Function[chan Out, Supplier[error]], retryConfig RetryConfig, errorHandler ...Consumer[error]) Supplier[Out] {
	return func() Out {
		resChan := make(chan Out, 0)
		theFunc := funcFactory(resChan)
		go HandleErrorAsync(func() error { return Retry(func() error { return theFunc() }, retryConfig) }, errorHandler...)

		return <-resChan
	}
}

func RetriableFunctionWithConfig[In, Out any](funcFactory Function[chan Out, Function[In, error]], retryConfig RetryConfig, errorHandler ...Consumer[error]) Function[In, Out] {
	return func(in In) Out {
		resChan := make(chan Out, 0)
		theFunc := funcFactory(resChan)
		go HandleErrorAsync(func() error { return Retry(func() error { return theFunc(in) }, retryConfig) }, errorHandler...)

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

func WaitForTheConditionToBecomeTrue[T any](resProvider func() T, predicate func(T) bool) T {
	var res T

	for res = resProvider(); !predicate(res); {
		res = resProvider()
	}
	return res
}

type RetryConfig struct {
	TimesRetry          int
	DelayBetweenRetries time.Duration
}

var defaultConfig = RetryConfig{10, 100 * time.Millisecond}

func Retry(theFunc Supplier[error], conf ...RetryConfig) error {
	currConf := defaultConfig
	if conf != nil && conf[0].TimesRetry > 0 {
		currConf = conf[0]
	}

	for i := 0; i < currConf.TimesRetry; i++ {
		err := theFunc()
		if err != nil && currConf.DelayBetweenRetries > 0 {
			if i == currConf.TimesRetry-1 {
				return err
			}
			time.Sleep(currConf.DelayBetweenRetries)
		}

	}
	return nil

}
