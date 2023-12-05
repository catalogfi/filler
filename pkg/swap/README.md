# Test setup
All tests can be run locally without internet connections. You'll need to setup local nodes
for different chains before running the tests. 

1. Bitcoin

We use [nigiri](https://nigiri.vulpem.com/) to simulate a local bitcoin node and indexer. 
```shell
$ nigiri start
```
2. Ethereum

We use [ganache](https://github.com/trufflesuite/ganache) to simulate a local ethereum node. 
```shell
ganache -b 1
```
# Env file

The unit tests are using env variables for some values. It's recommended to have a `.env` file to keep all the environment variables.
You'll only need to source this `.env` file before running the tests. You can find an example below
```text
BTC_USER="admin1"
BTC_PASSWORD="123"
BTC_INDEXER_ELECTRS_REGNET="http://0.0.0.0:30000"

ETH_URL="http://127.0.0.1:8545"
ETH_KEY_1="0x8c6cec18a7807ecfff68c588d8ff827198ae56ffdfb0d2639b4cd5b621931a57"
ETH_KEY_2="0xb7939b18c5ec570d1c2db12f68399b6e61eaa2f7c5ec5dc8c6765271b72dc167"
```
> The `ETH_KEY_1` and `ETH_KEY_2` needs to be updated from ganache output

# Test usage

```shell
ginkgo -v 

# Add the `cover` flag to get code coverage stats,
ginkgo -v --cover 
```