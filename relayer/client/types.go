package client

import (
	"context"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/relayer/v2/relayer/chains/cosmos"
	"github.com/cosmos/relayer/v2/relayer/provider"
	"main/concurrent"
	"main/funcs"
	"main/relayer/client/paths"
	"main/relayer/client/state"
	"main/utils"
	"sync"
)

type ChainClient struct {
	address         string
	chain           *cosmos.CosmosProvider
	lock            *sync.Mutex
	ctx             context.Context
	pathEnd         *paths.PathEnd
	cdc             codec.Codec
	chainState      *state.ChainState
	ibcHeaderLoader funcs.Function[int64, provider.IBCHeader]

	// todo get rid of these
	processedClientUpdates map[int64]bool
	latestCpHeight         int64
}

func ibcHeaderLoaderFactory(ctx context.Context, cosmosProvider *cosmos.CosmosProvider) funcs.Function[int64, provider.IBCHeader] {
	factory := func(channel chan provider.IBCHeader) funcs.Function[int64, error] {
		return func(height int64) error {
			hdr, err := cosmosProvider.QueryIBCHeader(ctx, height)
			if err != nil {
				return err
			}
			channel <- hdr
			return nil
		}
	}
	return funcs.RetryCallAndReturn[int64, provider.IBCHeader](factory)
}
func NewChainClient(ctx context.Context, cdc *codec.ProtoCodec, chain *cosmos.CosmosProvider, pathEnd *paths.PathEnd, latestHeight, latestCpHeight *concurrent.Future[int64]) *ChainClient {

	addr, err := chain.Address()
	utils.HandleError(err)
	cd := &ChainClient{
		address:                addr,
		chain:                  chain,
		ctx:                    ctx,
		pathEnd:                pathEnd,
		cdc:                    cdc,
		lock:                   &sync.Mutex{},
		processedClientUpdates: make(map[int64]bool),
		chainState:             state.NewChainState(ctx, cdc, chain, pathEnd, latestHeight.Get()),
		latestCpHeight:         latestCpHeight.Get(),
		ibcHeaderLoader:        ibcHeaderLoaderFactory(ctx, chain),
	}
	return cd
}

func (cd *ChainClient) Loader() *state.Loader {
	return cd.chainState.ForLatestHeight()
}

func (cd *ChainClient) MaybePrependUpdateClientAndSend(cpIBCHeaderSupplier func(int64) provider.IBCHeader, messageSupplier func() provider.RelayerMessage, cb funcs.BiConsumer[*provider.RelayerTxResponse, *state.Loader]) {
	cd.createUpdateClientMsgAndSend(cd.latestCpHeight, cpIBCHeaderSupplier, func(message provider.RelayerMessage) {
		if message == nil {
			cd.SendMessage(cd.ctx, messageSupplier(), "", cb)
		} else {
			cd.SendMessages(cd.ctx, []provider.RelayerMessage{message, messageSupplier()}, "", cb)
		}
	})

}

func (cd *ChainClient) createUpdateClientMsgAndSend(cpHeight int64, cpIBCHeaderSupplier func(int64) provider.IBCHeader, respond funcs.Consumer[provider.RelayerMessage]) {

	cd.lock.Lock()
	defer cd.lock.Unlock()
	if _, ok := cd.processedClientUpdates[cpHeight]; ok || cpHeight <= cd.latestCpHeight {

		respond(nil)
		return
	}
	cs := cd.chainState.ForLatestHeight().WithClientState().Load().ClientState()()

	cd.latestCpHeight = cpHeight

	if int64(cs.Val().LatestHeight.RevisionHeight) == cpHeight+1 {
		respond(nil)
		return
	}
	trustedHeader := concurrent.SupplyAsync[provider.IBCHeader](func() provider.IBCHeader {
		return cpIBCHeaderSupplier(int64(cs.Val().LatestHeight.RevisionHeight))
	})
	latestHeader := concurrent.SupplyAsync[provider.IBCHeader](func() provider.IBCHeader {
		return cpIBCHeaderSupplier(cpHeight + 1)
	})
	hdr, err := cd.chain.MsgUpdateClientHeader(latestHeader.Get(), cs.Val().LatestHeight, trustedHeader.Get())
	utils.HandleError(err)
	cuMsg, err := cd.chain.MsgUpdateClient(cd.pathEnd.ClientId(), hdr)
	utils.HandleError(err)
	cd.processedClientUpdates[cpHeight] = true
	respond(cuMsg)
}

func (cd *ChainClient) MaybeUpdateClient(height int64, cpIBCHeaderSupplier func(int64) provider.IBCHeader) {

	cd.createUpdateClientMsgAndSend(height, cpIBCHeaderSupplier, func(message provider.RelayerMessage) {
		if message != nil {
			cd.SendMessage(cd.ctx, message, "")
		}
	})

}

func (cd *ChainClient) IBCHeader(height int64) provider.IBCHeader {
	return cd.ibcHeaderLoader(height)
}

func (cd *ChainClient) stateBuilderFor(newHeight int64) *state.Loader {
	return cd.chainState.ForHeight(newHeight)
}

func (cd *ChainClient) SendMessage(ctx context.Context, msg provider.RelayerMessage, memo string, cb ...func(*provider.RelayerTxResponse, *state.Loader)) {
	resp, _, err := cd.chain.SendMessages(ctx, []provider.RelayerMessage{msg}, memo)
	utils.HandleError(err)
	if cb != nil {
		cb[0](resp, cd.stateBuilderFor(resp.Height))
	}
}

func (cd *ChainClient) SendMessages(ctx context.Context, msgs []provider.RelayerMessage, memo string, cb ...funcs.BiConsumer[*provider.RelayerTxResponse, *state.Loader]) {
	resp, _, err := cd.chain.SendMessages(ctx, msgs, memo)
	utils.HandleError(err)
	if cb != nil {
		cb[0](resp, cd.stateBuilderFor(resp.Height))
	}
}

func (cd *ChainClient) Address() string {
	return cd.address
}
