# VAAS Localnet Setup

This guide explains how to run a local VAAS (Validator-as-a-Service) testnet:
a provider chain, a consumer chain, and the IBC v2 relayer (`ts-relayer`) that
connects them.

For protocol details, see [`docs/consumer-lifecycle.md`](../docs/consumer-lifecycle.md).

## Prerequisites

- Go (version pinned in [`go.mod`](../go.mod))
- Docker: the relayer runs as a container; the image is pulled from
  `ghcr.io/allinbits/ibc-v2-ts-relayer:latest`.

## Building the Binaries

```bash
make build-apps                        # provider and consumer into build/
make build-apps MIN_VOTING_PERIOD=30s  # with the governance voting-period floor lowered
```

Both binaries carry their build information (`./build/provider version --long`):
the `git describe` output as the version, the commit hash, and the binary's
name.

atomone-sdk fixes the minimum governance voting period at link time (504h,
21 days) and enforces it in `genesis validate` and in every governance
`MsgUpdateParams`; a chain starts with any voting period, because `InitGenesis`
does not validate the parameters, so the refusal shows up later. A network
whose voting period is below the floor must build its binaries with
`MIN_VOTING_PERIOD` set at or below that period (the default, empty, keeps the
floor). The voting period also bounds four other gov fields that the same
checks enforce: `quorum_timeout` must be strictly below it,
`max_voting_period_extension` at least the voting period minus the quorum
timeout, and both deposit throttlers' `update_period` at most the voting
period. A short voting period is therefore set together with them; for 300s:

```json
"gov": {
  "params": {
    "voting_period": "300s",
    "quorum_timeout": "0s",
    "max_voting_period_extension": "300s",
    "min_deposit_throttler": { "update_period": "300s" },
    "min_initial_deposit_throttler": { "update_period": "300s" }
  }
}
```

`make provider-start` writes this block into the localnet genesis and builds
the binaries with `MIN_VOTING_PERIOD=300s`.

## Quick Start

A single command starts the entire localnet (provider, consumer, and relayer):

```bash
make localnet-start
```

This will:

1. Build the `provider` and `consumer` binaries into `build/`.
2. Start the provider chain (`provider-localnet`) in the background and wait
   for it to produce blocks.
3. Initialize and start the consumer chain (`consumer-localnet`) in the
   background, registering it on the provider with an immediate `spawn_time`
   and fetching its consumer genesis automatically.
4. Start the `ts-relayer` Docker container, fund relayer keys on both chains
   (test mnemonic), and create an **IBC v2 path** between the two chains
   (`add-path ... --ibc-version 2`).
5. Declare the relayer-created clients as the consumer's owner
   (`localnet-declare-clients`): the provider-side client through
   `update-consumer` (`client_id`), the consumer-side one through
   `set-provider-client`. The provider serves VSC packets over the declared
   client from the next epoch boundary and sends nothing before.
6. Fund the consumer's fee pool (`provider-fund-consumer-fee-pool`, from
   `val`): the provider collects each consumer's epoch fee from its pool, and
   a consumer whose pool cannot pay is flagged in debt and restricts user
   transactions until an epoch is paid again.
7. Trigger a delegation on the provider so the first VSC packet is queued and
   relayed to the consumer.

Log files:

- Provider: `/tmp/vaas-provider.log`
- Consumer: `/tmp/vaas-consumer.log`
- Relayer: `/tmp/vaas-ts-relayer.log` (plus `docker logs vaas-ts-relayer`)

## Manual Setup (3-Terminal)

For debugging or development, run each component in a separate terminal.

### Terminal 1: Provider Chain

```bash
make provider-start
```

Wait until you see blocks being produced before continuing.

### Terminal 2: Consumer Chain

```bash
# After provider is running
make consumer-start
```

`consumer-start` runs `consumer-init`, registers the consumer on the provider
(`consumer-create`), polls until the consumer genesis is available, patches
the local consumer `genesis.json`, copies the validator key from the provider
(single-validator localnet), and starts the consumer node.

### Terminal 3: Relayer

```bash
# After both chains are running
make ts-relayer-start
```

This launches the `vaas-ts-relayer` Docker container, registers test keys on
both chains, and creates the IBC v2 path. Stop it later with
`make ts-relayer-stop`.

### Declare the clients

```bash
# After the relayer has created its clients (a few seconds after add-path)
make localnet-declare-clients
```

The relayer only creates the IBC v2 clients; neither chain adopts one on its
own. `localnet-declare-clients` waits for the two clients to appear and then,
as the consumer's owner, declares the provider-side client through
`tx vaasprovider update-consumer` (its `client_id` field) and pins the
consumer-side client through `tx vaasconsumer set-provider-client`. Until the
provider-side declaration lands, the provider keeps its VSC packets queued;
until the consumer's pin lands, the consumer rejects every packet that arrives.

## Address Prefixes

Both binaries configure the `cosmos` bech32 prefixes by default. A deployment
that wants its own sets `VAAS_BECH32_PREFIX` in the binary's environment
(`VAAS_BECH32_PREFIX=shard consumer start ...`); the validator and consensus
prefixes derive from it. A value that is set but empty, or that is not a
lowercase letter followed by lowercase letters and digits other than `1`,
stops the binary at start-up. The provider and a consumer normally run under
different prefixes, and the protocol never relies on them matching: addresses
cross from one chain to the other as raw bytes, except the consumer's
`owner_address`, which the provider seeds into the consumer genesis as a
bech32 string under its own prefix and the consumer decodes without regard to
the prefix. The e2e suites run the consumer under `shard` for that reason.

## Network Configuration

| Chain    | RPC             | gRPC            | P2P             |
| -------- | --------------- | --------------- | --------------- |
| Provider | localhost:26657 | localhost:9090  | localhost:26656 |
| Consumer | localhost:26667 | localhost:9092  | localhost:26666 |

## Useful Commands

### Query Provider

```bash
# List consumer chains
./build/provider --home ~/.provider-localnet query vaasprovider list-consumer-chains

# Get consumer genesis (built by the provider at launch)
./build/provider --home ~/.provider-localnet query vaasprovider consumer-genesis 0
```

### Query Consumer

```bash
# Check provider info on the consumer
./build/consumer --home ~/.consumer-localnet query vaasconsumer provider-info

# Inspect IBC v2 clients (the consumer learns of the provider via its IBC client)
./build/consumer --home ~/.consumer-localnet query ibc client states
```

### Inspect the Relayer

```bash
# Follow the captured config output
tail -f /tmp/vaas-ts-relayer.log

# Live container logs
docker logs -f vaas-ts-relayer
```

## Clean Up

Stop all processes and remove all localnet data:

```bash
make localnet-clean
```

This kills the provider/consumer processes, removes the `ts-relayer`
container, deletes chain data directories (`~/.provider-localnet`,
`~/.consumer-localnet`), log files, and `/tmp/vaas-test/`.

## Troubleshooting

### Consumer fails to start with "consumer genesis not found"

The provider needs time to process the `MsgCreateConsumer` transaction and
reach the consumer's `spawn_time`. `make consumer-start` already includes a
retry loop; if it still fails, check that the consumer was registered:

```bash
./build/provider --home ~/.provider-localnet query vaasprovider list-consumer-chains

# Re-fetch and re-run manually
make consumer-genesis CONSUMER_ID=0
make consumer-run
```

### Relayer can't find a path / no VSC packets arriving

The provider does not create the IBC v2 client to the consumer itself: the
relayer does, and the provider sends nothing until the consumer's owner has
declared that client (`make localnet-declare-clients`, which `localnet-start`
runs as step 3b). Once declared, VSC packets go out at the next epoch boundary
(`blocks_per_epoch=10` in localnet). If you started the relayer but no VSC
packet arrives within ~30s after a delegation:

```bash
# Confirm ts-relayer is running
docker ps --filter name=vaas-ts-relayer

# Confirm the IBC v2 path was registered
docker exec vaas-ts-relayer /bin/with_keyring ibc-v2-ts-relayer list-paths

# Confirm IBC v2 clients exist on both ends
./build/provider --home ~/.provider-localnet query ibc client states
./build/consumer --home ~/.consumer-localnet query ibc client states

# Confirm the declarations landed: client_id on the provider's record,
# provider-info on the consumer
./build/provider --home ~/.provider-localnet query vaasprovider consumer-chain 0
./build/consumer --home ~/.consumer-localnet query vaasconsumer provider-info
```

### Port conflicts

If ports are already in use, find the offending process:

```bash
lsof -i :26657  # Provider RPC
lsof -i :26667  # Consumer RPC
lsof -i :9090   # Provider gRPC
lsof -i :9092   # Consumer gRPC
```

## Architecture

```text
   ┌─────────────────┐        IBC v2 (clients)         ┌─────────────────┐
   │                 │<────── VSC packets ────────────>│                 │
   │    Provider     │                                 │    Consumer     │
   │     chain       │       ts-relayer (Docker)       │     chain       │
   │  vaasprovider   │<───────────────────────────────>│  vaasconsumer   │
   │  app ID         │                                 │  app ID         │
   └─────────────────┘                                 └─────────────────┘
        :26657                                              :26667
```

VAAS is IBC v2 only. There is no channel handshake. Instead, the relayer
creates an IBC v2 client on each side pointing at the counterparty chain and
registers the counterparty (`add-path`). The consumer's owner then declares
the clients, once each: the provider-side one through `MsgUpdateConsumer`'s
`client_id` (`SetConsumerClient`), the consumer-side one through
`MsgSetProviderClient`. The provider sends VSC packets over the declared
client from the next epoch boundary (`SendVSCPackets`); packets are
out-of-order and deduplicated on the consumer via `HighestValsetUpdateID`.
