package client

import (
	"context"
	"github.com/cosmos/cosmos-sdk/codec"
	clienttypes "github.com/cosmos/ibc-go/v8/modules/core/02-client/types"
	chantypes "github.com/cosmos/ibc-go/v8/modules/core/04-channel/types"
	host "github.com/cosmos/ibc-go/v8/modules/core/24-host"
	ibcexported "github.com/cosmos/ibc-go/v8/modules/core/exported"
	tmclient "github.com/cosmos/ibc-go/v8/modules/light-clients/07-tendermint"
	"github.com/cosmos/relayer/v2/relayer/chains/cosmos"
	"github.com/cosmos/relayer/v2/relayer/provider"
	"main/utils"
	"math"
	"sync"
)

type ChainClient struct {
	address                string
	chain                  *cosmos.CosmosProvider
	lock                   *sync.Mutex
	ctx                    context.Context
	pathEnd                *PathEnd
	cdc                    codec.Codec
	latestCpHeight         int64
	chainStatePerHeight    map[int64]*ChainState
	processedClientUpdates map[int64]bool
}

type ChainState struct {
	height            int64
	chanProofData     *ProofData[chantypes.Channel]
	upgradeProofData  *ProofData[chantypes.Upgrade]
	latestClientState *tmclient.ClientState
}

func (cs *ChainState) ChanProofData() *ProofData[chantypes.Channel] {
	return cs.chanProofData
}

func (cs *ChainState) UpgradeProofData() *ProofData[chantypes.Upgrade] {
	return cs.upgradeProofData
}

func (cs *ChainState) LatestClientState() *tmclient.ClientState {
	return cs.latestClientState
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

func NewChainClient(ctx context.Context, cdc *codec.ProtoCodec, chain *cosmos.CosmosProvider, pathEnd *PathEnd) *ChainClient {

	addr, err := chain.Address()
	utils.HandleError(err)
	utils.HandleError(err)
	cd := &ChainClient{
		address:                addr,
		chain:                  chain,
		ctx:                    ctx,
		pathEnd:                pathEnd,
		chainStatePerHeight:    make(map[int64]*ChainState),
		cdc:                    cdc,
		lock:                   &sync.Mutex{},
		processedClientUpdates: make(map[int64]bool),
	}
	utils.HandleError(cd.updateToLatest())
	return cd
}

func (cd *ChainClient) updateToLatest() error {
	latestHeight, err := cd.chain.QueryLatestHeight(cd.ctx)

	if err != nil {
		return err
	}
	cd.MaybeUpdateChainState(latestHeight)
	return nil
}
func (cd *ChainClient) Height() int64 {

	return cd.pathEnd.Height()

}
func (cd *ChainClient) MaybePrependUpdateClientAndSend(height int64, cpIBCHeaderSupplier func(int64) provider.IBCHeader, messageSupplier func() provider.RelayerMessage, cb func(*provider.RelayerTxResponse)) {
	cd.createUpdateClientMsgAndSend(height, cpIBCHeaderSupplier, func(message provider.RelayerMessage) {
		if message == nil {
			cd.SendMessage(cd.ctx, messageSupplier(), "", cb)
		} else {
			cd.SendMessages(cd.ctx, []provider.RelayerMessage{message, messageSupplier()}, "", cb)
		}
	})

}

func (cd *ChainClient) createUpdateClientMsgAndSend(height int64, cpIBCHeaderSupplier func(int64) provider.IBCHeader, msgConsumer func(provider.RelayerMessage)) {
	cd.lock.Lock()
	defer cd.lock.Unlock()
	if _, ok := cd.processedClientUpdates[height]; ok || height <= cd.latestCpHeight {

		msgConsumer(nil)
		return
	}

	// todo cache
	chainState := cd.chainStatePerHeight[cd.pathEnd.Height()]
	trustedHeader := utils.NewFuture[provider.IBCHeader](func() provider.IBCHeader {
		return cpIBCHeaderSupplier(int64(chainState.latestClientState.LatestHeight.RevisionHeight))
	})
	latestHeader := utils.NewFuture[provider.IBCHeader](func() provider.IBCHeader {
		return cpIBCHeaderSupplier(height + 1)
	})
	hdr, err := cd.chain.MsgUpdateClientHeader(latestHeader.Get(), chainState.latestClientState.LatestHeight, trustedHeader.Get())
	utils.HandleError(err)
	cuMsg, err := cd.chain.MsgUpdateClient(cd.pathEnd.clientId, hdr)
	utils.HandleError(err)
	cd.latestCpHeight = height
	cd.processedClientUpdates[height] = true
	msgConsumer(cuMsg)
}

func (cd *ChainClient) MaybeUpdateClient(height int64, cpIBCHeaderSupplier func(int64) provider.IBCHeader) {

	cd.createUpdateClientMsgAndSend(height, cpIBCHeaderSupplier, func(message provider.RelayerMessage) {
		if message != nil {
			_, _, err := cd.chain.SendMessage(cd.ctx, message, "")
			utils.HandleError(err)
		}
	})

}

func (cd *ChainClient) IBCHeader(height int64) provider.IBCHeader {
	for {
		hdr, err := cd.chain.QueryIBCHeader(cd.ctx, height)
		if err == nil {
			return hdr
		}
	}
}

func (cd *ChainClient) MaybeUpdateChainState(newHeight int64) {

	cd.maybeUpdateChainState(newHeight)
}

func (cd *ChainClient) maybeUpdateChainState(newHeight int64) *ChainState {
	cd.lock.Lock()
	defer cd.lock.Unlock()

	// todo generalize this
	chanProofSupplier := utils.NewFuture[*ProofData[chantypes.Channel]](func() *ProofData[chantypes.Channel] {
		if cd.pathEnd.ChanId() == "" {
			return nil
		}

		var val, proof []byte
		var err error
		var proofHeight clienttypes.Height
		for val, proof, proofHeight, err = cd.chain.QueryTendermintProof(cd.ctx, newHeight+1, host.ChannelKey(cd.pathEnd.Port(), cd.pathEnd.ChanId())); err != nil; {
			val, proof, proofHeight, err = cd.chain.QueryTendermintProof(cd.ctx, newHeight+1, host.ChannelKey(cd.pathEnd.Port(), cd.pathEnd.ChanId()))
		}
		theChannel := chantypes.Channel{}
		utils.HandleError(cd.cdc.Unmarshal(val, &theChannel))
		return &ProofData[chantypes.Channel]{val: &theChannel, proof: proof, height: proofHeight}
	})

	upgradeProofSupplier := utils.NewFuture[*ProofData[chantypes.Upgrade]](func() *ProofData[chantypes.Upgrade] {
		if !cd.pathEnd.Upgrade() {
			return nil
		}

		var val, proof []byte
		var err error
		var proofHeight clienttypes.Height
		for val, proof, proofHeight, err = cd.chain.QueryTendermintProof(cd.ctx, newHeight+1, host.ChannelUpgradeKey(cd.pathEnd.Port(), cd.pathEnd.ChanId())); err != nil; {
			val, proof, proofHeight, err = cd.chain.QueryTendermintProof(cd.ctx, newHeight+1, host.ChannelUpgradeKey(cd.pathEnd.Port(), cd.pathEnd.ChanId()))
		}
		theUpgrade := chantypes.Upgrade{}

		utils.HandleError(cd.cdc.Unmarshal(val, &theUpgrade))
		return &ProofData[chantypes.Upgrade]{val: &theUpgrade, proof: proof, height: proofHeight}
	})
	clientStateSupplier := utils.NewFuture[*tmclient.ClientState](func() *tmclient.ClientState {
		var st ibcexported.ClientState
		var err error
		for st, err = cd.chain.QueryClientState(cd.ctx, newHeight+1, cd.pathEnd.clientId); err != nil; {
			st, err = cd.chain.QueryClientState(cd.ctx, newHeight+1, cd.pathEnd.clientId)
		}
		return st.(*tmclient.ClientState)
	})
	cd.pathEnd.SetHeight(int64(math.Max(float64(newHeight), float64(cd.pathEnd.Height()))))
	theState := &ChainState{latestClientState: clientStateSupplier.Get(), chanProofData: chanProofSupplier.Get(), upgradeProofData: upgradeProofSupplier.Get(), height: newHeight}
	cd.chainStatePerHeight[newHeight] = theState
	return theState
}

func (cd *ChainClient) GetChainStateForHeight(height int64) *ChainState {
	return cd.getChainStateForHeight(height)
}

func (cd *ChainClient) getChainStateForHeight(height int64) *ChainState {
	return cd.maybeUpdateChainState(height)

}

func (cd *ChainClient) SendMessage(ctx context.Context, msg provider.RelayerMessage, memo string, cb func(*provider.RelayerTxResponse)) {
	resp, _, err := cd.chain.SendMessages(ctx, []provider.RelayerMessage{msg}, memo)
	utils.HandleError(err)
	cd.pathEnd.SetHeight(int64(math.Max(float64(resp.Height), float64(cd.pathEnd.Height()))))
	cb(resp)
}

func (cd *ChainClient) SendMessages(ctx context.Context, msgs []provider.RelayerMessage, memo string, cb func(*provider.RelayerTxResponse)) {
	resp, _, err := cd.chain.SendMessages(ctx, msgs, memo)
	utils.HandleError(err)
	cd.pathEnd.SetHeight(int64(math.Max(float64(resp.Height), float64(cd.pathEnd.Height()))))
	cb(resp)
}

func (cd *ChainClient) Address() string {
	return cd.address
}

type PathEnd struct {
	clientId string
	connId   string
	port     string
	chanId   string
	upgrade  bool
	height   int64
	lock     *sync.Mutex
}

func NewPathEnd(client, port, connection, channel string, upgrade bool) *PathEnd {
	return &PathEnd{
		lock:     new(sync.Mutex),
		port:     port,
		connId:   connection,
		clientId: client,
		chanId:   channel,
		upgrade:  upgrade,
	}
}

func (pe *PathEnd) Height() int64 {
	pe.lock.Lock()
	defer pe.lock.Unlock()

	return pe.height
}

func (pe *PathEnd) SetHeight(height int64) {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	pe.height = height
}
func (pe *PathEnd) ClientId() string {

	pe.lock.Lock()
	defer pe.lock.Unlock()

	return pe.clientId
}
func (pe *PathEnd) SetClientId(clientId string) {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	pe.clientId = clientId
}
func (pe *PathEnd) ConnId() string {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	return pe.connId
}
func (pe *PathEnd) SetConnId(connId string) {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	pe.connId = connId
}
func (pe *PathEnd) Port() string {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	return pe.port
}
func (pe *PathEnd) SetPort(port string) {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	pe.port = port
}
func (pe *PathEnd) ChanId() string {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	return pe.chanId
}
func (pe *PathEnd) SetChanId(chanId string) {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	pe.chanId = chanId
}

func (pe *PathEnd) Upgrade() bool {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	return pe.upgrade
}

func (pe *PathEnd) SetUpgrade(upgrade bool) {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	pe.upgrade = upgrade
}

type Path struct {
	source *PathEnd
	dest   *PathEnd
}

type Props struct {
	SourceChannel    string
	SourceClient     string
	SourcePort       string
	SourceConnection string
	SourceUpgrade    bool
	DestChannel      string
	DestClient       string
	DestPort         string
	DestConnection   string
	DestUpgrade      bool
}

func NewPath(props *Props) *Path {
	return &Path{
		source: NewPathEnd(props.SourceClient, props.SourcePort, props.SourceConnection, props.SourceChannel, props.SourceUpgrade),
		dest:   NewPathEnd(props.DestClient, props.DestPort, props.DestConnection, props.DestChannel, props.DestUpgrade),
	}
}

func (p *Path) Source() *PathEnd {
	return p.source
}
func (p *Path) Dest() *PathEnd {
	return p.dest
}
