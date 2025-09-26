## A little Go library for interaction with IBC-Go


This project was born out of a neccessity to overperfom Go [relayer](https://github.com/cosmos/relayer) for the purpose of exploiting following [vulnurability](https://hackerone.com/reports/2917368). Since the vulnarability is based on race condition I needed to develop a faster relayer. 

And here main optimization comes from concurrent proof retrieval and client updates. In the original relayer version, client updates are prepended to whatever messages we send. Here, client updates are initiated right after the change on the counterparty [chain](https://github.com/unknownfeature/ibc-tools/blob/master/relayer/types.go#L166). And while the update is constructed, we assemble the next message and maybe prepend a client update message to it or maybe not if it has been already [sent](https://github.com/unknownfeature/ibc-tools/blob/master/relayer/client/types.go#L93).

