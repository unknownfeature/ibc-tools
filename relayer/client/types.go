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
	"main/relayer/client/paths"
	"main/relayer/client/state"
	"main/utils"
	"math"
	"sync"
)

type ChainClient struct {
	address                string
	chain                  *cosmos.CosmosProvider
	lock                   *sync.Mutex
	ctx                    context.Context
	pathEnd                *paths.PathEnd
	cdc                    codec.Codec
	latestCpHeight         int64
	chainStatePerHeight    map[int64]*state.ChainState
	processedClientUpdates map[int64]bool
}

func NewChainClient(ctx context.Context, cdc *codec.ProtoCodec, chain *cosmos.CosmosProvider, pathEnd *paths.PathEnd) *ChainClient {

	addr, err := chain.Address()
	utils.HandleError(err)
	utils.HandleError(err)
	cd := &ChainClient{
		address:                addr,
		chain:                  chain,
		ctx:                    ctx,
		pathEnd:                pathEnd,
		chainStatePerHeight:    make(map[int64]*state.ChainState),
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
	cuMsg, err := cd.chain.MsgUpdateClient(cd.pathEnd.ClientId(), hdr)
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

func (cd *ChainClient) maybeUpdateChainState(newHeight int64) *state.ChainState {
	cd.lock.Lock()
	defer cd.lock.Unlock()

	// todo generalize this
	chanProofSupplier := utils.NewFuture[*state.ProofData[chantypes.Channel]](func() *state.ProofData[chantypes.Channel] {
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
		return &state.ProofData[chantypes.Channel]{val: &theChannel, proof: proof, height: proofHeight}
	})

	upgradeProofSupplier := utils.NewFuture[*state.ProofData[chantypes.Upgrade]](func() *state.ProofData[chantypes.Upgrade] {
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
		return &state.ProofData[chantypes.Upgrade]{val: &theUpgrade, proof: proof, height: proofHeight}
	})
	clientStateSupplier := utils.NewFuture[*tmclient.ClientState](func() *tmclient.ClientState {
		var st ibcexported.ClientState
		var err error
		for st, err = cd.chain.QueryClientState(cd.ctx, newHeight+1, cd.pathEnd.ClientId()); err != nil; {
			st, err = cd.chain.QueryClientState(cd.ctx, newHeight+1, cd.pathEnd.ClientId())
		}
		return st.(*tmclient.ClientState)
	})
	cd.pathEnd.SetHeight(int64(math.Max(float64(newHeight), float64(cd.pathEnd.Height()))))
	theState := &state.ChainState{latestClientState: clientStateSupplier.Get(), chanProofData: chanProofSupplier.Get(), upgradeProofData: upgradeProofSupplier.Get(), height: newHeight}
	cd.chainStatePerHeight[newHeight] = theState
	return theState
}

func (cd *ChainClient) GetChainStateForHeight(height int64) *state.ChainState {
	return cd.getChainStateForHeight(height)
}

func (cd *ChainClient) getChainStateForHeight(height int64) *state.ChainState {
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
