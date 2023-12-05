Cobi 
- executor 
- rpc server 
- bot

core pkgs
- store 
   - keep track of order secrets
   - store the order details locally
   - optional (store the instant wallet secret)
- swap 
  - Bitcoin/Ethereum atomic swap
- wallet  
  - manage the private key and concurrent transactions 
  - instant wallet integration
  - can initialise/redeem/refund a swap
  

1. Store
   - keep track of order secrets
   - store the order details locally 
   - optional (store the instant wallet secret) 
2. Swap
   - Bitcoin/Ethereum atomic swap 
3. Wallet (Key/Account management, virtual balance)
   - Instant wallet integration
   - Initialise/redeem/refund a swap 
4. Executor 
   - Listening orders from the orderbook through websockets 
   - Executor the orders when necessary 
4. JSON-RPC server 
   - Listening for requests to set up the daemon 

6. Bot
   - Auto creating orders 
   - Auto filling orders 


