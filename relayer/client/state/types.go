package state

import (
	"context"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/gogoproto/proto"
	clienttypes "github.com/cosmos/ibc-go/v8/modules/core/02-client/types"
	connectiontypes "github.com/cosmos/ibc-go/v8/modules/core/03-connection/types"
	chantypes "github.com/cosmos/ibc-go/v8/modules/core/04-channel/types"
	host "github.com/cosmos/ibc-go/v8/modules/core/24-host"
	tmclient "github.com/cosmos/ibc-go/v8/modules/light-clients/07-tendermint"
	"github.com/cosmos/relayer/v2/relayer/chains/cosmos"
	"main/relayer/client/paths"
	"main/utils"
	"math"
	"sync"
	"time"
)

var defaultCacheTTL = time.Second * 60

func readTendermintProofFunctionFactory[T any](ctx context.Context, chainProvider *cosmos.CosmosProvider, keySupplier utils.Supplier[[]byte], transformer utils.Function[[]byte, *T]) utils.Function[int64, ProofData[T]] {
	return func(height int64) ProofData[T] {
		var val, proof []byte
		var err error
		var proofHeight clienttypes.Height
		for val, proof, proofHeight, err = chainProvider.QueryTendermintProof(ctx, height+1, keySupplier()); err != nil; {
			val, proof, proofHeight, err = chainProvider.QueryTendermintProof(ctx, height+1, keySupplier())
		}

		return ProofData[T]{val: transformer(val), proof: proof, height: proofHeight}

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
func transformerForCreator[T proto.Message](cdc codec.Codec, creator utils.Supplier[T]) utils.Function[[]byte, T] {
	return func(bytes []byte) T {
		res := creator()
		utils.HandleError(cdc.Unmarshal(bytes, res))
		return res
	}
}

func noopTransformer[T *[]byte](t []byte) T {
	return &t
}

func clientStateKeySupplier(pathEnd *paths.PathEnd) utils.Supplier[[]byte] {
	return func() []byte { return host.FullClientStateKey(pathEnd.ClientId()) }
}
func connectionKeySupplier(pathEnd *paths.PathEnd) utils.Supplier[[]byte] {
	return func() []byte { return host.ConnectionKey(pathEnd.ConnId()) }
}
func channelKeySupplier(pathEnd *paths.PathEnd) utils.Supplier[[]byte] {
	return func() []byte { return host.ChannelKey(pathEnd.Port(), pathEnd.ChanId()) }
}

func packetCommitmentKeySupplier(pathEnd *paths.PathEnd) utils.Supplier[[]byte] {
	return func() []byte { return host.PacketCommitmentKey(pathEnd.Port(), pathEnd.ChanId(), pathEnd.Seq()) }
}
func packetReceiptKeySupplier(pathEnd *paths.PathEnd) utils.Supplier[[]byte] {
	return func() []byte { return host.PacketReceiptKey(pathEnd.Port(), pathEnd.ChanId(), pathEnd.Seq()) }
}
func packetAcknowledgementKeySupplier(pathEnd *paths.PathEnd) utils.Supplier[[]byte] {
	return func() []byte { return host.PacketAcknowledgementKey(pathEnd.Port(), pathEnd.ChanId(), pathEnd.Seq()) }
}
func upgradeKeySupplier(pathEnd *paths.PathEnd) utils.Supplier[[]byte] {
	return func() []byte { return host.ChannelKey(pathEnd.Port(), pathEnd.ChanId()) }
}

type State struct {
	channelState               utils.Supplier[*ProofData[chantypes.Channel]]
	upgradeState               utils.Supplier[*ProofData[chantypes.Upgrade]]
	clientState                utils.Supplier[*ProofData[tmclient.ClientState]]
	connectionState            utils.Supplier[*ProofData[connectiontypes.ConnectionEnd]]
	packetCommitmentState      utils.Supplier[*ProofData[[]byte]]
	packetReceiptState         utils.Supplier[*ProofData[[]byte]]
	packetAcknowledgementState utils.Supplier[*ProofData[[]byte]]
}

func (s *State) Channel() utils.Supplier[*ProofData[chantypes.Channel]] {
	return s.channelState
}
func (s *State) Upgrade() utils.Supplier[*ProofData[chantypes.Upgrade]] {
	return s.upgradeState
}
func (s *State) ClientState() utils.Supplier[*ProofData[tmclient.ClientState]] {
	return s.clientState
}
func (s *State) ConnectionState() utils.Supplier[*ProofData[connectiontypes.ConnectionEnd]] {
	return s.connectionState
}

func (s *State) PacketCommitmentState() utils.Supplier[*ProofData[[]byte]] {
	return s.packetCommitmentState
}

func (s *State) PacketReceiptState() utils.Supplier[*ProofData[[]byte]] {
	return s.packetReceiptState
}

func (s *State) PacketAcknowledgementState() utils.Supplier[*ProofData[[]byte]] {
	return s.packetAcknowledgementState
}

type ForHeightBuilder struct {
	channelState               *utils.Future[*ProofData[chantypes.Channel]]
	upgradeState               *utils.Future[*ProofData[chantypes.Upgrade]]
	clientState                *utils.Future[*ProofData[tmclient.ClientState]]
	connectionState            *utils.Future[*ProofData[connectiontypes.ConnectionEnd]]
	packetCommitmentState      *utils.Future[*ProofData[[]byte]]
	packetReceiptState         *utils.Future[*ProofData[[]byte]]
	packetAcknowledgementState *utils.Future[*ProofData[[]byte]]
	cs                         *ChainState
	height                     int64
	sealed                     bool
	lock                       *sync.Mutex
}
)
func (b *ForHeightBuilder) WithChannelState() *ForHeightBuilder {

	return b.doInLockIfNotSealedAndIf(func() { b.channelState = b.cs.channelStateManager.Get(b.height) }, b.channelState == nil)
}

func (b *ForHeightBuilder) WithUpgradeState() *ForHeightBuilder {

	return b.doInLockIfNotSealedAndIf(func() { b.upgradeState = b.cs.upgradeStateManager.Get(b.height) }, b.upgradeState == nil)
}

func (b *ForHeightBuilder) WithClientState() *ForHeightBuilder {

	return b.doInLockIfNotSealedAndIf(func() { b.clientState = b.cs.clientStateManager.Get(b.height) }, b.clientState == nil)
}
func (b *ForHeightBuilder) WithConnectionState() *ForHeightBuilder {

	return b.doInLockIfNotSealedAndIf(func() { b.connectionState = b.cs.connectionStateManager.Get(b.height) }, b.connectionState == nil)
}

func (b *ForHeightBuilder) WithPacketCommitmentStatee() *ForHeightBuilder {

	return b.doInLockIfNotSealedAndIf(func() { b.packetCommitmentState = b.cs.packetCommitmentStateManager.Get(b.height) }, b.packetCommitmentState == nil)

}
func (b *ForHeightBuilder) WithPacketReceiptStatee() *ForHeightBuilder {

	return b.doInLockIfNotSealedAndIf(func() { b.packetReceiptState = b.cs.packetReceiptStateManager.Get(b.height) }, b.packetReceiptState == nil)

}

func (b *ForHeightBuilder) WithPacketAcknowledgementStatee() *ForHeightBuilder {

	return b.doInLockIfNotSealedAndIf(func() { b.packetAcknowledgementState = b.cs.packetAcknowledgementStateManager.Get(b.height) },
		b.packetAcknowledgementState == nil)

}

func (b *ForHeightBuilder) doInLockIfNotSealedAndIf(whatToDo func(), condition bool) *ForHeightBuilder {
	b.lock.Lock()
	defer b.lock.Unlock()
	if b.sealed || !condition {
		return b
	}
	whatToDo()
	return b
}
func (b *ForHeightBuilder) Build() *State {
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

func NewChainState(ctx context.Context, cdc codec.Codec, chainProvider *cosmos.CosmosProvider, end *paths.PathEnd) *ChainState {
	return &ChainState{

		lock:                              &sync.Mutex{},
		channelStateManager:               newStateManager(defaultCacheTTL, readTendermintProofFunctionFactory(ctx, chainProvider, channelKeySupplier(end), transformerForCreator(cdc, channelCreator))),
		upgradeStateManager:               newStateManager(defaultCacheTTL, readTendermintProofFunctionFactory(ctx, chainProvider, upgradeKeySupplier(end), transformerForCreator(cdc, upgradeCreator))),
		clientStateManager:                newStateManager(defaultCacheTTL, readTendermintProofFunctionFactory(ctx, chainProvider, clientStateKeySupplier(end), transformerForCreator(cdc, clientStateCreator))),
		connectionStateManager:            newStateManager(defaultCacheTTL, readTendermintProofFunctionFactory(ctx, chainProvider, connectionKeySupplier(end), transformerForCreator(cdc, connectionStateCreator))),
		packetCommitmentStateManager:      newStateManager[[]byte](defaultCacheTTL, readTendermintProofFunctionFactory[[]byte](ctx, chainProvider, packetCommitmentKeySupplier(end), noopTransformer[*[]byte])),
		packetReceiptStateManager:         newStateManager[[]byte](defaultCacheTTL, readTendermintProofFunctionFactory[[]byte](ctx, chainProvider, packetReceiptKeySupplier(end), noopTransformer[*[]byte])),
		packetAcknowledgementStateManager: newStateManager[[]byte](defaultCacheTTL, readTendermintProofFunctionFactory[[]byte](ctx, chainProvider, packetAcknowledgementKeySupplier(end), noopTransformer[*[]byte])),
	}

}

func (cs *ChainState) ForHeight(height int64) *ForHeightBuilder {
	cs.lock.Lock()
	cs.height = int64(math.Max(float64(height), float64(cs.height)))
	cs.lock.Unlock()
	return &ForHeightBuilder{cs: cs, height: height, lock: &sync.Mutex{}}
}

func (cs *ChainState) ForLatestHeight() *ForHeightBuilder {
	cs.lock.Lock()
	cs.lock.Unlock()
	return &ForHeightBuilder{cs: cs, height: cs.height, lock: &sync.Mutex{}}
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
	stateCache   *utils.ExpiringConcurrentMap[int64, ProofData[T]]
	newStateFunc utils.Function[int64, ProofData[T]]
}

func newStateManager[T any](ttl time.Duration, stateFunc utils.Function[int64, ProofData[T]]) *Manager[T] {

	return &Manager[T]{
		stateCache:   utils.NewExpiresAfterDurationConcurrentMap[int64, ProofData[T]](ttl),
		newStateFunc: stateFunc,
	}
}

func (sk Manager[T]) Get(height int64) *utils.Future[*ProofData[T]] {
	return utils.NewFuture[*ProofData[T]](func() *ProofData[T] {
		res := sk.stateCache.ComputeIfAbsent(height, sk.newStateFunc)
		return &res
	})
}
