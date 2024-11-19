package relayer

import (
	"fmt"
	"time"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	clienttypes "github.com/cosmos/ibc-go/v9/modules/core/02-client/types"
	ibcexported "github.com/cosmos/ibc-go/v9/modules/core/exported"
	tmclient "github.com/cosmos/ibc-go/v9/modules/light-clients/07-tendermint"
)

type ClientStateInfo struct {
	ChainID        string
	TrustingPeriod time.Duration
	LatestHeight   ibcexported.Height
	UnbondingTime  time.Duration
}

func ClientInfoFromClientState(clientState *codectypes.Any) (ClientStateInfo, error) {
	clientStateExported, err := clienttypes.UnpackClientState(clientState)
	if err != nil {
		return ClientStateInfo{}, err
	}

	switch t := clientStateExported.(type) {
	case *tmclient.ClientState:
		return ClientStateInfo{
			ChainID:        t.ChainId,
			TrustingPeriod: t.TrustingPeriod,
			LatestHeight:   t.LatestHeight,
			UnbondingTime:  t.UnbondingPeriod,
		}, nil
	default:
		return ClientStateInfo{}, fmt.Errorf("unhandled client state type: (%T)", clientState)
	}
}
