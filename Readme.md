## COBI: Your Decentralized Liquidity Provider

- COBI is a distributed software solution designed to manage liquidity provision written in Go (golang). It functions as an opposite bidder in atomic swaps for a catalog orderbook. COBI allows users to run the software on their machines, providing a seamless experience in managing their liquidity and wallets.

### Components of Cobi Codebase

COBI is inspired by the architectural design of the [btcsuite](https://github.com/btcsuite) Golang implementation. It consists of two main components: `cobid` and `cobi-cli`.

- `cobid` operates as a daemon-like service that initiates a background RPC server. This server is used to manage executors, fillers, and creators along with wallet functions.

- `cobi-cli` is a command-line interface (CLI) RPC client used to communicate with cobi-d through JSON-RPC with basic Authentication. It provides a user-friendly interface for interacting with the COBI system, managing their wallets.

## Table of contents

- Installation and Setup
- Getting Started

## Installation and Setup Guide

Install cobi with following guide

### prerequisites

```
Make
git
golang
```

- clone this repositery

```bash
  git clone https://github.com/catalogfi/cobi
```

- checkout into required branch current branch is `v1-iw`
- use `Make` to build your binaries

```
make build_deamons
```

- This creates your binaries in ~/.cobi directory for `rpc` ,` strategies` and `executor`

Now In order to Get started with Daemon you can run following command

```
make start
```

- this command will spin up a Json Rpc Daemon ready for you to interact with

You can further docs for interaction with Json Rpc Daemon, defining your strategiesm operating executors at [cobid docs](docs/cobid.md)

## Interacting With Cobi-cli

Make sure you set cobi-cli bin in your environment

```bash
export PATH="$PATH~/.cobi/bin/cobi-cli"
```

You could check if its exported and built correctly using following command

```bash
cobi-cli --help
```

For further interaction docs with cobi-cli refer [cli docs](docs/cli.md)

> folder Structure

```
├── cli
│   ├── cli.go
│   └── commands
│       ├── accounts.go
│       ├── create.go
│       ...
├── cmd
│   ├── cli
│   │   └── main.go
│   └── daemon
│       ├── executor
│       │   └── main.go
│       ├── rpc
│       │   └── main.go
│       └── strategy
│           └── main.go
├── daemon
│   ├── executor
│   │   └── executor.go
│   ├── rpc
│   │   ├── handlers
│   │   │   ├── accounts.go
│   │   │   ├── create.go
│   │   │   ├── ...
│   │   │   └── types.go
│   │   ├── methods
│   │   │   └── methods.go
│   │   └── rpc.go
│   ├── strategy
│   │   ├── strategy.go
│   │   └── types.go
│   └── types
│       └── types.go
├── go.mod
├── go.sum
├── jsonRpc_collection.json
├── Makefile
├── pkg
│   ├── blockchain
│   │   └── blockchain.go
│   └── swapper
│       ├── bitcoin
│       │   ├── bitcoin.go
│       │   ├── client.go
│       │   ├── indexer.go
│       │   ├── instant.go
│       │   ├── store.go
│       │   ├── swap.go
│       │   └── watcher.go
│       ├── ethereum
│       │   ├── client.go
│       │   ├── contracts
│       │   │   ├── ...
│       │   ├── ethereum.go
│       │   ├── typings
│       │   │   ├── ...
│       │   └── watcher.go
│       └── swapper.go
│
├── rpcclient
│   ├── client.go
│   ├── client_suite_test.go
│   └── client_test.go
├── store
│   └── store.go
|
└── utils
    ├── key.go
    └── sys.go

```

- `cmd` : has entrypoints to start(daemon process) and build the respective services
- `cli` : contains all `cobra` cli commands. which can be used by user to communicate with json-rpc daemon
  - `commands` : cobra Commands abstracted into different file. these files resemble files in `handlers` which indicate one-on-one logic mapping for server logic in our architecture.
- `daemon` : core services of cobid background processes
  - `executor` : this `daemon` process runs in background with responsibility of executing its part of atomicSwap trade based on websocket events recieved from websocket from `Catalog Orderbook`
  - `rpc` : json-rpc server which handles and operates executors and fillers. `rpc` is also used to do wallet specific tasks and handling users instantWallets
    - `handlers` : rpc handlers to operate and interact with daemon efficiently
  - `strategy` : these are daemon process responsible for running autofill and autoCreate logic for your defined strategies
- `pkg` : consists of `blockchain` and `Swapper` packages responsible for handling chain specific logic and atomicswap execution logic respectively
- `rpcclient` : json rpc client which implements all functions and Basic Auth for json rpc server

## API Reference

- Server follows JSON RPC pattern [version 2]

### Base Request Payload Structure

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "", # METHOD
  "params": { } # BODY JSON
}
```

#### Get all accounts , balances , and virtual balances

Method : getAccountInfo

| Parameter       | Type     | Description                                        |
| :-------------- | :------- | :------------------------------------------------- |
| `userAccount`   | `uint`   | **Required**. Account user wants to get balance of |
| `orderPair`     | `string` | **Required** `fromChain:fromAsset-ToChain:ToAsset` |
| `sendAmount`    | `string` | **Required** amount to be sent                     |
| `receiveAmount` | `string` | **Required** amount to be expected in return       |

Method : createNewOrder

| Parameter       | Type     | Description                                        |
| :-------------- | :------- | :------------------------------------------------- |
| `userAccount`   | `uint`   | **Required**. Account user wants to get balance of |
| `orderPair`     | `string` | **Required** `fromChain:fromAsset-ToChain:ToAsset` |
| `sendAmount`    | `string` | **Required** amount to be sent                     |
| `receiveAmount` | `string` | **Required** amount to be expected in return       |

Method : fillOrder

| Parameter     | Type   | Description                                        |
| :------------ | :----- | :------------------------------------------------- |
| `userAccount` | `uint` | **Required**. Account user wants to get balance of |
| `orderId`     | `uint` | **Required**. Order ID to be filled                |

Method : getAccountInfo

| Parameter     | Type     | Description                                            |
| :------------ | :------- | :----------------------------------------------------- |
| `userAccount` | `uint`   | **Required**. Account user wants to get balance of     |
| `perPage`     | `uint`   | Number of accounts per page                            |
| `asset`       | `string` | Asset to filter accounts                               |
| `page`        | `uint`   | Page number                                            |
| `isLegacy`    | `bool`   | Flag to indicate if legacy accounts should be included |

Method : depositFunds

| Parameter     | Type     | Description                                       |
| :------------ | :------- | :------------------------------------------------ |
| `userAccount` | `uint`   | **Required**. IW Account user wants to deposit to |
| `asset`       | `string` | **Required**. Asset to be deposited               |
| `amount`      | `string` | **Required**. Amount to be deposited              |

Method : transferFunds

| Parameter     | Type     | Description                                          |
| :------------ | :------- | :--------------------------------------------------- |
| `userAccount` | `uint`   | **Required**. Account user wants to transfer from    |
| `asset`       | `string` | **Required**. Asset to be transferred                |
| `amount`      | `string` | **Required**. Amount to be transferred               |
| `toAddr`      | `string` | **Required**. Destination address                    |
| `useIw`       | `bool`   | Flag to indicate if Instant wallet should be used    |
| `force`       | `bool`   | Flag to force the transfer [ignores virtual balance] |

Method setCoreConfig

| Parameter  | Type     | Description                                  |
| :--------- | :------- | :------------------------------------------- |
| `Mnemonic` | `string` | **Required**. Mnemonic for the configuration |
| `TBD`      |          |                                              |
