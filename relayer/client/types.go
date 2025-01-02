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
	"math"
	"sync"
)

func IBCHeaderLoaderFactory(ctx context.Context, cosmosProvider *cosmos.CosmosProvider) funcs.Function[chan provider.IBCHeader, funcs.Function[int64, error]] {
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
	return factory
}

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

func NewChainClient(ctx context.Context, cdc *codec.ProtoCodec, chain *cosmos.CosmosProvider, pathEnd *paths.PathEnd, latestHeight, latestCpHeight *concurrent.Future[int64]) *ChainClient {

	addr, err := chain.Address()
	funcs.HandleError(err)
	cd := &ChainClient{
		address:                addr,
		chain:                  chain,
		ctx:                    ctx,
		pathEnd:                pathEnd,
		cdc:                    cdc,
		lock:                   &sync.Mutex{},
		processedClientUpdates: make(map[int64]bool),
		chainState:             state.NewChainState(ctx, cdc, chain, pathEnd, *latestHeight.Get()),
		latestCpHeight:         *latestCpHeight.Get(),
		ibcHeaderLoader:        funcs.RetriableFunctionWithConfig[int64, provider.IBCHeader](IBCHeaderLoaderFactory(ctx, chain), funcs.RetryConfig{TimesRetry: math.MaxInt})}
	return cd
}

func (cd *ChainClient) Loader() *state.Loader {
	return cd.chainState.ForLatestHeight()
}

func (cd *ChainClient) MaybePrependUpdateClientAndSend(cpHeight int64, cpIBCHeaderSupplier func(int64) provider.IBCHeader, messageSupplier func() *provider.RelayerMessage, cb funcs.BiConsumer[*provider.RelayerTxResponse, *state.Loader]) {
	cd.createUpdateClientMsgAndSend(cpHeight, cpIBCHeaderSupplier, func(message *provider.RelayerMessage) {
		if message == nil {
			cd.SendMessage(cd.ctx, messageSupplier(), "", cb)
		} else {
			cd.SendMessages(cd.ctx, []provider.RelayerMessage{*message, *messageSupplier()}, "", cb)
		}
	})

}

func (cd *ChainClient) createUpdateClientMsgAndSend(cpHeight int64, cpIBCHeaderSupplier func(int64) provider.IBCHeader, respond funcs.Consumer[*provider.RelayerMessage]) {

	cd.lock.Lock()
	if _, ok := cd.processedClientUpdates[cpHeight]; ok {
		cd.lock.Unlock()
		respond(nil)
		return
	}
	cs := cd.chainState.ForLatestHeight().WithClientState().Load().ClientState()()

	cd.latestCpHeight = int64(math.Max(float64(cpHeight), float64(cd.latestCpHeight)))

	if int64(cs.Val().LatestHeight.RevisionHeight) >= cpHeight+1 {
		cd.lock.Unlock()
		respond(nil)
		return
	}
	trustedHeader := concurrent.SupplyAsync[provider.IBCHeader](func() provider.IBCHeader {
		return cpIBCHeaderSupplier(int64(cs.Val().LatestHeight.RevisionHeight))
	})
	latestHeader := concurrent.SupplyAsync[provider.IBCHeader](func() provider.IBCHeader {
		return cpIBCHeaderSupplier(cpHeight + 1)
	})
	hdr, err := cd.chain.MsgUpdateClientHeader(*latestHeader.Get(), cs.Val().LatestHeight, *trustedHeader.Get())
	funcs.HandleError(err)
	cuMsg, err := cd.chain.MsgUpdateClient(cd.pathEnd.ClientId(), hdr)
	funcs.HandleError(err)
	cd.processedClientUpdates[cpHeight] = true
	cd.lock.Unlock()
	respond(&cuMsg)
}

func (cd *ChainClient) MaybeUpdateClient(height int64, cpIBCHeaderSupplier func(int64) provider.IBCHeader) {

	cd.createUpdateClientMsgAndSend(height, cpIBCHeaderSupplier, func(message *provider.RelayerMessage) {
		if message != nil {
			cd.SendMessage(cd.ctx, message, "")
		}
	})

}

func (cd *ChainClient) IBCHeader(height int64) provider.IBCHeader {
	return cd.ibcHeaderLoader(height)
}

func (cd *ChainClient) stateLoaderFor(newHeight int64) *state.Loader {
	return cd.chainState.ForHeight(newHeight)
}

func (cd *ChainClient) SendMessage(ctx context.Context, msg *provider.RelayerMessage, memo string, cb ...func(*provider.RelayerTxResponse, *state.Loader)) {
	resp, _, err := cd.chain.SendMessages(ctx, []provider.RelayerMessage{*msg}, memo)
	funcs.HandleError(err)
	loader := cd.stateLoaderFor(resp.Height).WithClientState()
	if cb != nil {
		cb[0](resp, loader)
	}
}

func (cd *ChainClient) SendMessages(ctx context.Context, msgs []provider.RelayerMessage, memo string, cb ...funcs.BiConsumer[*provider.RelayerTxResponse, *state.Loader]) {
	resp, _, err := cd.chain.SendMessages(ctx, msgs, memo)
	funcs.HandleError(err)
	loader := cd.stateLoaderFor(resp.Height).WithClientState()
	if cb != nil {
		cb[0](resp, loader)
	}
}

func (cd *ChainClient) Address() string {
	return cd.address
}
