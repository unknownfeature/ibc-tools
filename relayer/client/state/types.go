package state

import (
	"context"
	"fmt"
	"github.com/avast/retry-go"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/gogoproto/proto"
	clienttypes "github.com/cosmos/ibc-go/v8/modules/core/02-client/types"
	connectiontypes "github.com/cosmos/ibc-go/v8/modules/core/03-connection/types"
	chantypes "github.com/cosmos/ibc-go/v8/modules/core/04-channel/types"
	host "github.com/cosmos/ibc-go/v8/modules/core/24-host"
	ibcexported "github.com/cosmos/ibc-go/v8/modules/core/exported"
	tmclient "github.com/cosmos/ibc-go/v8/modules/light-clients/07-tendermint"
	"github.com/cosmos/relayer/v2/relayer/chains/cosmos"
	concurrent2 "main/concurrent"
	"main/funcs"
	"main/relayer/client/paths"
	"main/utils"
	"math"
	"sync"
	"time"
)

var defaultCacheTTL = time.Second * 60

func retriableProofLoader[T any](defaultProofLoadingFunctionFactory funcs.Function[chan ProofData[T], funcs.Function[int64, error]]) funcs.Function[int64, ProofData[T]] {
	return func(height int64) ProofData[T] {
		resChan := make(chan ProofData[T], 0)
		proofLoader := defaultProofLoadingFunctionFactory(resChan)
		utils.HandleError(retry.Do(func() error { return proofLoader(height) }))

		return <-resChan
	}
}

func defaultProofLoadingFunctionFactory[T any](ctx context.Context, chainProvider *cosmos.CosmosProvider, keySupplier funcs.Supplier[[]byte], transformer funcs.Function[[]byte, *T]) funcs.Function[chan ProofData[T], funcs.Function[int64, error]] {
	return func(resChan chan ProofData[T]) funcs.Function[int64, error] {
		return func(height int64) error {
			val, proof, proofHeight, err := chainProvider.QueryTendermintProof(ctx, height+1, keySupplier())
			if err != nil {
				return err
			}
			resChan <- ProofData[T]{val: transformer(val), proof: proof, height: proofHeight}
			return nil
		}
	}
}

func clientSateProofLoadingFunctionFactory(ctx context.Context, chainProvider *cosmos.CosmosProvider, pathEnd *paths.PathEnd) funcs.Function[chan ProofData[tmclient.ClientState], funcs.Function[int64, error]] {
	return retriableProofLoader[tmclient.ClientState](func(resChan chan ProofData[tmclient.ClientState]) funcs.Function[int64, error] {
		return func(height int64) error {
			st, err := chainProvider.QueryClientState(ctx, height+1, pathEnd.ClientId())
			if err != nil {
				return err
			}
			resChan <- ProofData[tmclient.ClientState]{val: st.(*tmclient.ClientState), proof: nil, height: clienttypes.Height{RevisionHeight: uint64(height)}}
			return nil
		}
	})

}

func readTendermintProofFunctionFactory[T any](ctx context.Context, chainProvider *cosmos.CosmosProvider, keySupplier funcs.Supplier[[]byte], transformer funcs.Function[[]byte, *T]) funcs.Function[int64, ProofData[T]] {
	return retriableProofLoader[T](defaultProofLoadingFunctionFactory[T](ctx, chainProvider, keySupplier, transformer))
}

func clientStateFunction(ctx context.Context, chainProvider *cosmos.CosmosProvider, pathEnd *paths.PathEnd) funcs.Function[int64, ProofData[tmclient.ClientState]] {
	return func(height int64) ProofData[tmclient.ClientState] {
		var st ibcexported.ClientState
		var err error
		for st, err = chainProvider.QueryClientState(ctx, height+1, pathEnd.ClientId()); err != nil; {
			st, err = chainProvider.QueryClientState(ctx, height+1, pathEnd.ClientId())
		}
		return ProofData[tmclient.ClientState]{val: st.(*tmclient.ClientState), proof: nil, height: clienttypes.Height{RevisionHeight: uint64(height)}}
	}
}
func channelCreator() *chantypes.Channel {
	return &chantypes.Channel{}
}
func upgradeCreator() *chantypes.Upgrade {
	return &chantypes.Upgrade{}
}

func clientStateCreator() *tmclient.ClientState {
	return &tmclient.ClientState{}
}

func connectionStateCreator() *connectiontypes.ConnectionEnd {
	return &connectiontypes.ConnectionEnd{}
}
func transformerForCreator[T proto.Message](cdc codec.Codec, creator funcs.Supplier[T]) funcs.Function[[]byte, T] {
	return func(bytes []byte) T {
		res := creator()
		utils.HandleError(cdc.Unmarshal(bytes, res))
		return res
	}
}

func noopTransformer[T *[]byte](t []byte) T {
	return &t
}

func clientStateKeySupplier(pathEnd *paths.PathEnd) funcs.Supplier[[]byte] {
	return func() []byte { return host.FullClientStateKey(pathEnd.ClientId()) }
}
func connectionKeySupplier(pathEnd *paths.PathEnd) funcs.Supplier[[]byte] {
	return func() []byte { return host.ConnectionKey(pathEnd.ConnId()) }
}
func channelKeySupplier(pathEnd *paths.PathEnd) funcs.Supplier[[]byte] {
	return func() []byte { return host.ChannelKey(pathEnd.Port(), pathEnd.ChanId()) }
}

func packetCommitmentKeySupplier(pathEnd *paths.PathEnd) funcs.Supplier[[]byte] {
	return func() []byte { return host.PacketCommitmentKey(pathEnd.Port(), pathEnd.ChanId(), pathEnd.Seq()) }
}
func packetReceiptKeySupplier(pathEnd *paths.PathEnd) funcs.Supplier[[]byte] {
	return func() []byte { return host.PacketReceiptKey(pathEnd.Port(), pathEnd.ChanId(), pathEnd.Seq()) }
}
func packetAcknowledgementKeySupplier(pathEnd *paths.PathEnd) funcs.Supplier[[]byte] {
	return func() []byte { return host.PacketAcknowledgementKey(pathEnd.Port(), pathEnd.ChanId(), pathEnd.Seq()) }
}
func upgradeKeySupplier(pathEnd *paths.PathEnd) funcs.Supplier[[]byte] {
	return func() []byte { return host.ChannelUpgradeKey(pathEnd.Port(), pathEnd.ChanId()) }
}

type State struct {
	channelState               funcs.Supplier[*ProofData[chantypes.Channel]]
	upgradeState               funcs.Supplier[*ProofData[chantypes.Upgrade]]
	clientState                funcs.Supplier[*ProofData[tmclient.ClientState]]
	connectionState            funcs.Supplier[*ProofData[connectiontypes.ConnectionEnd]]
	packetCommitmentState      funcs.Supplier[*ProofData[[]byte]]
	packetReceiptState         funcs.Supplier[*ProofData[[]byte]]
	packetAcknowledgementState funcs.Supplier[*ProofData[[]byte]]
}

func (s *State) Channel() funcs.Supplier[*ProofData[chantypes.Channel]] {
	return s.channelState
}
func (s *State) Upgrade() funcs.Supplier[*ProofData[chantypes.Upgrade]] {
	return s.upgradeState
}
func (s *State) ClientState() funcs.Supplier[*ProofData[tmclient.ClientState]] {
	return s.clientState
}
func (s *State) ConnectionState() funcs.Supplier[*ProofData[connectiontypes.ConnectionEnd]] {
	return s.connectionState
}

func (s *State) PacketCommitmentState() funcs.Supplier[*ProofData[[]byte]] {
	return s.packetCommitmentState
}

func (s *State) PacketReceiptState() funcs.Supplier[*ProofData[[]byte]] {
	return s.packetReceiptState
}

func (s *State) PacketAcknowledgementState() funcs.Supplier[*ProofData[[]byte]] {
	return s.packetAcknowledgementState
}

type StateLoader struct {
	channelState               *concurrent2.Future[*ProofData[chantypes.Channel]]
	upgradeState               *concurrent2.Future[*ProofData[chantypes.Upgrade]]
	clientState                *concurrent2.Future[*ProofData[tmclient.ClientState]]
	connectionState            *concurrent2.Future[*ProofData[connectiontypes.ConnectionEnd]]
	packetCommitmentState      *concurrent2.Future[*ProofData[[]byte]]
	packetReceiptState         *concurrent2.Future[*ProofData[[]byte]]
	packetAcknowledgementState *concurrent2.Future[*ProofData[[]byte]]
	cs                         *ChainState
	height                     int64
	sealed                     bool
	lock                       *sync.Mutex
}

func (b *StateLoader) WithChannelState() *StateLoader {

	return b.doInLockIfNotSealedAndIf(func() { b.channelState = b.cs.channelStateManager.Get(b.height) }, b.channelState == nil)
}

func (b *StateLoader) WithUpgradeState() *StateLoader {

	return b.doInLockIfNotSealedAndIf(func() { b.upgradeState = b.cs.upgradeStateManager.Get(b.height) }, b.upgradeState == nil)
}

func (b *StateLoader) WithClientState() *StateLoader {

	return b.doInLockIfNotSealedAndIf(func() { b.clientState = b.cs.clientStateManager.Get(b.height) }, b.clientState == nil)
}
func (b *StateLoader) WithConnectionState() *StateLoader {

	return b.doInLockIfNotSealedAndIf(func() { b.connectionState = b.cs.connectionStateManager.Get(b.height) }, b.connectionState == nil)
}

func (b *StateLoader) WithPacketCommitmentStatee() *StateLoader {

	return b.doInLockIfNotSealedAndIf(func() { b.packetCommitmentState = b.cs.packetCommitmentStateManager.Get(b.height) }, b.packetCommitmentState == nil)

}
func (b *StateLoader) WithPacketReceiptStatee() *StateLoader {

	return b.doInLockIfNotSealedAndIf(func() { b.packetReceiptState = b.cs.packetReceiptStateManager.Get(b.height) }, b.packetReceiptState == nil)

}

func (b *StateLoader) WithPacketAcknowledgementStatee() *StateLoader {

	return b.doInLockIfNotSealedAndIf(func() { b.packetAcknowledgementState = b.cs.packetAcknowledgementStateManager.Get(b.height) },
		b.packetAcknowledgementState == nil)

}

func (b *StateLoader) doInLockIfNotSealedAndIf(whatToDo func(), condition bool) *StateLoader {
	b.lock.Lock()
	defer b.lock.Unlock()
	if b.sealed || !condition {
		return b
	}
	whatToDo()
	return b
}
func (b *StateLoader) Load() *State {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.sealed = true
	return &State{
		channelState:               b.channelState.Get,
		upgradeState:               b.upgradeState.Get,
		clientState:                b.clientState.Get,
		connectionState:            b.connectionState.Get,
		packetCommitmentState:      b.packetCommitmentState.Get,
		packetReceiptState:         b.packetReceiptState.Get,
		packetAcknowledgementState: b.packetAcknowledgementState.Get,
	}
}

type ChainState struct {
	height                            int64
	lock                              *sync.Mutex
	channelStateManager               *Manager[chantypes.Channel]
	upgradeStateManager               *Manager[chantypes.Upgrade]
	clientStateManager                *Manager[tmclient.ClientState]
	connectionStateManager            *Manager[connectiontypes.ConnectionEnd]
	packetCommitmentStateManager      *Manager[[]byte]
	packetReceiptStateManager         *Manager[[]byte]
	packetAcknowledgementStateManager *Manager[[]byte]
}

func NewChainState(ctx context.Context, cdc codec.Codec, chainProvider *cosmos.CosmosProvider, end *paths.PathEnd, height int64) *ChainState {

	return &ChainState{
		height:                            height,
		lock:                              &sync.Mutex{},
		channelStateManager:               newStateManager(defaultCacheTTL, readTendermintProofFunctionFactory(ctx, chainProvider, channelKeySupplier(end), transformerForCreator(cdc, channelCreator))),
		upgradeStateManager:               newStateManager(defaultCacheTTL, readTendermintProofFunctionFactory(ctx, chainProvider, upgradeKeySupplier(end), transformerForCreator(cdc, upgradeCreator))),
		clientStateManager:                newStateManager(defaultCacheTTL, clientStateFunction(ctx, chainProvider, end)),
		connectionStateManager:            newStateManager(defaultCacheTTL, readTendermintProofFunctionFactory(ctx, chainProvider, connectionKeySupplier(end), transformerForCreator(cdc, connectionStateCreator))),
		packetCommitmentStateManager:      newStateManager[[]byte](defaultCacheTTL, readTendermintProofFunctionFactory[[]byte](ctx, chainProvider, packetCommitmentKeySupplier(end), noopTransformer[*[]byte])),
		packetReceiptStateManager:         newStateManager[[]byte](defaultCacheTTL, readTendermintProofFunctionFactory[[]byte](ctx, chainProvider, packetReceiptKeySupplier(end), noopTransformer[*[]byte])),
		packetAcknowledgementStateManager: newStateManager[[]byte](defaultCacheTTL, readTendermintProofFunctionFactory[[]byte](ctx, chainProvider, packetAcknowledgementKeySupplier(end), noopTransformer[*[]byte])),
	}

}

func (cs *ChainState) ForHeight(height int64) *StateLoader {
	cs.lock.Lock()
	cs.height = int64(math.Max(float64(height), float64(cs.height)))
	fmt.Println("new height", height)
	cs.lock.Unlock()
	return &StateLoader{cs: cs, height: height, lock: &sync.Mutex{}}
}

func (cs *ChainState) ForLatestHeight() *StateLoader {
	cs.lock.Lock()
	cs.lock.Unlock()
	return &StateLoader{cs: cs, height: cs.height, lock: &sync.Mutex{}}
}
func (cs *ChainState) Height() int64 {
	cs.lock.Lock()
	cs.lock.Unlock()
	return cs.height
}

type ProofData[T any] struct {
	proof  []byte
	height clienttypes.Height
	val    *T
}

func (d *ProofData[T]) Proof() []byte {
	return d.proof
}

func (d *ProofData[T]) Height() clienttypes.Height {
	return d.height
}

func (d *ProofData[T]) Val() *T {
	return d.val
}

type Manager[T any] struct {
	stateCache   *concurrent2.ConcurrentMap[int64, ProofData[T]]
	newStateFunc funcs.Function[int64, ProofData[T]]
}

func newStateManager[T any](ttl time.Duration, stateFunc funcs.Function[int64, ProofData[T]]) *Manager[T] {

	return &Manager[T]{
		stateCache:   concurrent2.NewExpiresAfterDurationConcurrentMap[int64, ProofData[T]](ttl),
		newStateFunc: stateFunc,
	}
}

func (sk Manager[T]) Get(height int64) *concurrent2.Future[*ProofData[T]] {
	return concurrent2.SupplyAsync[*ProofData[T]](func() *ProofData[T] {
		res := sk.stateCache.ComputeIfAbsent(height, sk.newStateFunc)
		return &res
	})
}
