package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/relayer/v2/cmd"
	"github.com/cosmos/relayer/v2/relayer"
	"github.com/cosmos/relayer/v2/relayer/chains/cosmos"
	"go.uber.org/zap"
	"main/funcs"
	"os"
	"path/filepath"
)

type Props struct {
	Key        string
	ChainID    string
	HomePath   string
	Mnemonic   string
	ConfigRoot string
}

func NewProvider(ctx context.Context, props Props) *cosmos.CosmosProvider {
	chain, err := addChainFromFile(props.ChainID, fmt.Sprintf("%s.json", props.ChainID), props.ConfigRoot)
	funcs.HandleError(err)
	err = chain.ChainProvider.Init(ctx)
	funcs.HandleError(err)
	if props.Mnemonic != "" {
		_, err = chain.ChainProvider.RestoreKey(props.Key, props.Mnemonic, 118, string(hd.Secp256k1Type))
		funcs.HandleError(err)

	}
	err = chain.ChainProvider.UseKey(props.Key)
	funcs.HandleError(err)

	return chain.ChainProvider.(*cosmos.CosmosProvider)
}
func addChainFromFile(chainId, file, homePath string) (*relayer.Chain, error) {
	var pcw cmd.ProviderConfigWrapper
	if _, err := os.Stat(filepath.Join(homePath, file)); err != nil {
		funcs.HandleError(err)
	}

	byt, err := os.ReadFile(filepath.Join(homePath, file))
	funcs.HandleError(err)

	if err = json.Unmarshal(byt, &pcw); err != nil {
		return nil, err
	}

	logs := zap.NewExample()
	prov, err := pcw.Value.NewProvider(
		logs,
		homePath, true, chainId,
	)
	funcs.HandleError(err)
	c := relayer.NewChain(logs, prov, true)

	return c, nil
}
