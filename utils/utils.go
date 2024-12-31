package utils

import (
	"context"
	"encoding/json"
	"errors"
	clienttypes "github.com/cosmos/ibc-go/v8/modules/core/02-client/types"
	connectiontypes "github.com/cosmos/ibc-go/v8/modules/core/03-connection/types"
	chantypes "github.com/cosmos/ibc-go/v8/modules/core/04-channel/types"
	"github.com/cosmos/relayer/v2/relayer/provider"
	"io/fs"
	"os"
	"strconv"
	"sync"
)

func ReadSeedPhrase(path string) string {
	file, err := os.Open(path)
	HandleError(err)
	defer file.Close()

	decoder := json.NewDecoder(file)
	var data map[string]string
	HandleError(decoder.Decode(&data))
	return data["mnemonic"]
}

func Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func HandleError(err error) {
	if err != nil {
		panic(err)
	}
}

func ParseClientIDFromEvents(events []provider.RelayerEvent) string {
	for _, event := range events {
		if event.EventType == clienttypes.EventTypeCreateClient {
			for attributeKey, attributeValue := range event.Attributes {
				if attributeKey == clienttypes.AttributeKeyClientID {
					return attributeValue
				}
			}
		}
	}

	panic("client identifier event attribute not found")
}

func ParseConnectionIDFromEvents(events []provider.RelayerEvent) string {
	for _, event := range events {
		if event.EventType == connectiontypes.EventTypeConnectionOpenInit || event.EventType == connectiontypes.EventTypeConnectionOpenTry {
			for attributeKey, attributeValue := range event.Attributes {
				if attributeKey == connectiontypes.AttributeKeyConnectionID {
					return attributeValue
				}
			}
		}
	}
	panic("connection identifier event attribute not found")
}

func ParseChannelIDFromEvents(events []provider.RelayerEvent) string {
	for _, event := range events {
		if event.EventType == chantypes.EventTypeChannelOpenInit || event.EventType == chantypes.EventTypeChannelOpenTry {
			for attributeKey, attributeValue := range event.Attributes {
				if attributeKey == chantypes.AttributeKeyChannelID {
					return attributeValue
				}
			}
		}
	}
	panic("channel identifier event attribute not found")
}
func ParseSequenceFromEvents(events []provider.RelayerEvent) uint64 {
	for _, event := range events {
		//if event.EventType == chantypes.EventTypeChannelOpenInit || event.EventType == chantypes.EventTypeChannelOpenTry {
		for attributeKey, attributeValue := range event.Attributes {
			if attributeKey == chantypes.AttributeKeySequence {
				res, err := strconv.ParseUint(attributeValue, 10, 64)
				HandleError(err)
				return res
			}
		}
		//}
	}
	panic("channel identifier event attribute not found")
}

func DoInLock(lock *sync.Mutex, cb func()) {
	lock.Lock()
	defer lock.Unlock()
	cb()
}

func DoInLockAndReturn[T any](lock *sync.Mutex, cb Supplier[T]) T {
	lock.Lock()
	defer lock.Unlock()
	return cb()
}

func WaitForTheConditionToBecomeTrue[T any](resProvider func() T, predicate func(T) bool) T {
	var res T
	for res = resProvider(); !predicate(res); {
		res = resProvider()
	}
	return res
}

func WaitForTheConditionToBecomeTrueAndReturnTransformed[T, K any](resProvider func() T, predicate func(T) bool, transformer func(T) K) K {
	var res T
	for res = resProvider(); !predicate(res); {
		res = resProvider()
	}
	return transformer(res)
}
func WaitForTNoErrorAndReturn[T any](ctx context.Context, resProvider func(ctx context.Context) (T, error)) T {
	var res T
	var err error
	for res, err = resProvider(ctx); err != nil; {
		res, err = resProvider(ctx)
	}
	return res
}
func SubmitWithWaitGroup(task func(), wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		task()
	}()
}
