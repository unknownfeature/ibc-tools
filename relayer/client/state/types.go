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
	"main/concurrent"
	"main/funcs"
	"main/relayer/client/paths"
	"math"
	"sync"
)

func clientSateProofLoader(ctx context.Context, chainProvider *cosmos.CosmosProvider, pathEnd *paths.PathEnd) funcs.Function[int64, ProofData[tmclient.ClientState]] {
	factory := func(resChan chan ProofData[tmclient.ClientState]) funcs.Function[int64, error] {
		return func(height int64) error {
			st, err := chainProvider.QueryClientState(ctx, height+1, pathEnd.ClientId())
			if err != nil {
				return err
			}
			resChan <- ProofData[tmclient.ClientState]{val: st.(*tmclient.ClientState), proof: nil, height: clienttypes.Height{RevisionHeight: uint64(height)}}
			return nil
		}
	}
	return funcs.RetriableFunction(factory)

}

func tendermintProofLoader[T any](ctx context.Context, chainProvider *cosmos.CosmosProvider, keySupplier funcs.Supplier[[]byte], transformer funcs.Function[[]byte, *T]) funcs.Function[int64, ProofData[T]] {

	factory := func(resChan chan ProofData[T]) funcs.Function[int64, error] {
		return func(height int64) error {
			val, proof, proofHeight, err := chainProvider.QueryTendermintProof(ctx, height+1, keySupplier())
			if err != nil {
				return err
			}
			resChan <- ProofData[T]{val: transformer(val), proof: proof, height: proofHeight}
			return nil
		}
	}
	return funcs.RetriableFunction(factory)
}

func channelCreator() *chantypes.Channel {
	return &chantypes.Channel{}
}
func upgradeCreator() *chantypes.Upgrade {
	return &chantypes.Upgrade{}
}

func connectionStateCreator() *connectiontypes.ConnectionEnd {
	return &connectiontypes.ConnectionEnd{}
}
func transformerForCreator[T proto.Message](cdc codec.Codec, creator funcs.Supplier[T]) funcs.Function[[]byte, T] {
	return func(bytes []byte) T {
		res := creator()
		funcs.HandleError(cdc.Unmarshal(bytes, res))
		return res
	}
}

func noopTransformer[T *[]byte](t []byte) T {
	return &t
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
	height int64
}

func (s *State) Height() int64 {
	return s.height
}

type ClientState struct {
	State
	clientState funcs.Supplier[*ProofData[tmclient.ClientState]]
}

func (cs *ClientState) ClientStateSupplier() funcs.Supplier[*ProofData[tmclient.ClientState]] {
	return cs.clientState
}

type ChannelState struct {
	State
	channelState funcs.Supplier[*ProofData[chantypes.Channel]]
}

func (cs *ChannelState) ChannelStateSupplier() funcs.Supplier[*ProofData[chantypes.Channel]] {
	return cs.channelState
}

type ChannelUpgradeState struct {
	ChannelState
	upgradeState funcs.Supplier[*ProofData[chantypes.Upgrade]]
}

func (cs *ChannelUpgradeState) UpgradeStateSupplier() funcs.Supplier[*ProofData[chantypes.Upgrade]] {
	return cs.upgradeState
}

type ChainStateManager struct {
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

func NewChainState(ctx context.Context, cdc codec.Codec, chainProvider *cosmos.CosmosProvider, end *paths.PathEnd, height int64) *ChainStateManager {

	return &ChainStateManager{
		height:                            height,
		lock:                              &sync.Mutex{},
		channelStateManager:               newStateManager(tendermintProofLoader(ctx, chainProvider, channelKeySupplier(end), transformerForCreator(cdc, channelCreator))),
		upgradeStateManager:               newStateManager(tendermintProofLoader(ctx, chainProvider, upgradeKeySupplier(end), transformerForCreator(cdc, upgradeCreator))),
		clientStateManager:                newStateManager(clientSateProofLoader(ctx, chainProvider, end)),
		connectionStateManager:            newStateManager(tendermintProofLoader(ctx, chainProvider, connectionKeySupplier(end), transformerForCreator(cdc, connectionStateCreator))),
		packetCommitmentStateManager:      newStateManager[[]byte](tendermintProofLoader[[]byte](ctx, chainProvider, packetCommitmentKeySupplier(end), noopTransformer[*[]byte])),
		packetReceiptStateManager:         newStateManager[[]byte](tendermintProofLoader[[]byte](ctx, chainProvider, packetReceiptKeySupplier(end), noopTransformer[*[]byte])),
		packetAcknowledgementStateManager: newStateManager[[]byte](tendermintProofLoader[[]byte](ctx, chainProvider, packetAcknowledgementKeySupplier(end), noopTransformer[*[]byte])),
	}

}

func (cs *ChainStateManager) LoadClient(height int64) *ChainStateManager {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	cs.height = cs.maxHeight(height)
	cs.clientStateManager.Load(cs.height)
	return cs
}

func (cs *ChainStateManager) LoadChannel(height int64) *ChainStateManager {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	cs.height = cs.maxHeight(height)
	cs.channelStateManager.Load(cs.height)
	return cs
}

func (cs *ChainStateManager) LoadChannelUpgrade(height int64) *ChainStateManager {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	cs.height = cs.maxHeight(height)
	cs.channelStateManager.Load(cs.height)
	cs.upgradeStateManager.Load(cs.height)
	return cs
}

func (cs *ChainStateManager) LatestClient() *ClientState {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	return &ClientState{State{height: cs.height}, cs.clientStateManager.Get(cs.height).Get}
}

func (cs *ChainStateManager) LatestChannel() *ChannelState {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	return &ChannelState{State{height: cs.height}, cs.channelStateManager.Get(cs.height).Get}
}

func (cs *ChainStateManager) LatestChannelUpgrade() *ChannelUpgradeState {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	return &ChannelUpgradeState{ChannelState{State{height: cs.height}, cs.channelStateManager.Get(cs.height).Get}, cs.upgradeStateManager.Get(cs.height).Get}
}

func (cs *ChainStateManager) maxHeight(height int64) int64 {
	return int64(math.Max(float64(height), float64(cs.height)))
}

func (cs *ChainStateManager) Height() int64 {
	cs.lock.Lock()
	defer cs.lock.Unlock()
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
	stateCache   *concurrent.ConcurrentMap[int64, *concurrent.Future[ProofData[T]]]
	newStateFunc funcs.Function[int64, *concurrent.Future[ProofData[T]]]
}

func newStateManager[T any](loadFunction funcs.Function[int64, ProofData[T]]) *Manager[T] {

	return &Manager[T]{ // todo add purge
		stateCache: concurrent.NewConcurrentMap[int64, *concurrent.Future[ProofData[T]]](),
		newStateFunc: func(height int64) *concurrent.Future[ProofData[T]] {
			return concurrent.SupplyAsync(func() ProofData[T] { return loadFunction(height) })
		},
	}
}

func (sk Manager[T]) Get(height int64) *concurrent.Future[ProofData[T]] {
	return sk.stateCache.ComputeIfAbsent(height, sk.newStateFunc)
}

func (sk Manager[T]) Load(height int64) {
	sk.stateCache.PutIfAbsent(height, sk.newStateFunc)
}
