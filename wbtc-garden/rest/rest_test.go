package rest_test

// import (
// 	. "github.com/onsi/ginkgo/v2"
// 	. "github.com/onsi/gomega"
// )

// var (
// 	// s *rest.Server
// 	c              rest.Client
// 	jwtToken       string
// 	CurrentOrderID uint
// )

// var _ = BeforeSuite(func() {
// 	filePath := "gorm.db"
// 	if _, err := os.Stat(filePath); err == nil {
// 		// If file exists, then remove
// 		os.Remove(filePath)
// 	}

// 	// done := make(chan bool)
// 	StartServer()
// 	// <-done
// 	time.Sleep(3 * time.Second) // await server to start

// 	if os.Getenv("PRIVATE_KEY") == "" {
// 		panic("PRIVATE_KEY not set")
// 	}

// 	c = rest.NewClient("http://localhost:8080", os.Getenv("PRIVATE_KEY"))
// 	jwtToken = ""
// 	CurrentOrderID = 0
// })

// var _ = Describe("Rest", func() {
// 	It("check health of server", func() {
// 		Expect(c).NotTo(BeNil())
// 		Expect(jwtToken).To(Equal(""))
// 		Expect(CurrentOrderID).To(Equal(uint(0)))
// 		Expect(c.Health()).To(Equal("online"))
// 	})

// 	It("check nonce", func() {
// 		nonce, err := c.GetNonce()
// 		Expect(err).NotTo(HaveOccurred())
// 		Expect(nonce).ToNot(BeNil())
// 	})

// 	It("check verify", func() {
// 		verified, err := c.Login()
// 		Expect(err).NotTo(HaveOccurred())
// 		Expect(verified).ToNot(BeNil())
// 		c.SetJwt(verified)
// 	})

// 	It("should create Order", func() {
// 		// Skip("will test after price fetching logic implemented")
// 		OrderID, err := c.CreateOrder("mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet:primary-ethereum_sepolia:0x4FDAAe676608f2a768f9c57BFDAeFA7559283316", "1", "10", "d87c01599e0f31a714ca73e5de993e274430101d4675d80da19d84b2bf19817d")
// 		CurrentOrderID = OrderID
// 		Expect(err).NotTo(HaveOccurred())
// 		Expect(OrderID).To(BeNumerically(">=", 0))
// 	})

// 	It("fill the order", func() {
// 		// Skip("will test after price fetching logic implemented")
// 		err := c.FillOrder(CurrentOrderID, "0xF403cE7776B22B74EcA871EcDaBeAA2103CD4A49", "mxHKgg7dU4pt9abWXveMofqRvWr7f6xx7g")
// 		Expect(err).NotTo(HaveOccurred())
// 	})

// 	It("get Initiator Initiate Orders", func() {
// 		// Skip("will test after price fetching logic implemented")
// 		orders, err := c.GetInitiatorInitiateOrders()
// 		Expect(err).NotTo(HaveOccurred())
// 		Expect(orders).ToNot(BeNil())
// 		Expect(len(orders)).To(BeNumerically(">=", 1))
// 		CurrentOrder := orders[len(orders)-1]
// 		Expect(CurrentOrder.ID).To(Equal(CurrentOrderID))
// 		// Expect(CurrentOrder.Status).To(Equal("FILLED"))
// 		Expect(CurrentOrder.OrderPair).To(Equal("bitcoin_testnet:primary-ethereum_sepolia:0x4FDAAe676608f2a768f9c57BFDAeFA7559283316"))
// 		Expect(CurrentOrder.Taker).To(Equal(strings.ToLower("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da")))
// 		Expect(CurrentOrder.Maker).To(Equal(strings.ToLower("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da")))
// 		Expect(CurrentOrder.Status).To(Equal(model.Status(2)))
// 		// fmt.Println("CurrentOrder: ", CurrentOrder.Taker, CurrentOrder.Status, CurrentOrder.OrderPair)
// 	})

// 	It("get Followers init Orders", func() {
// 		// Skip("will test after price fetching logic implemented")
// 		orders, err := c.GetFollowerInitiateOrders()
// 		Expect(err).NotTo(HaveOccurred())
// 		Expect(orders).ToNot(BeNil())
// 		Expect(len(orders)).To(BeNumerically(">=", 0))

// 		// fmt.Println("CurrentOrder: ", CurrentOrder.Taker, CurrentOrder.Status, CurrentOrder.OrderPair)
// 	})

// 	It("get Initiator Redeem Orders", func() {
// 		// Skip("will test after price fetching logic implemented")
// 		order, err := c.GetInitiatorRedeemOrders()
// 		Expect(err).NotTo(HaveOccurred())
// 		Expect(order).ToNot(BeNil())
// 		Expect(len(order)).To(BeNumerically("==", 0)) // as atomic swap is not implemented yet
// 	})

// 	It("get Followers Redeem Orders", func() {
// 		// Skip("will test after price fetching logic implemented")
// 		order, err := c.GetFollowerRedeemOrders()
// 		Expect(err).NotTo(HaveOccurred())
// 		Expect(order).ToNot(BeNil())
// 		Expect(len(order)).To(BeNumerically("==", 0)) // as atomic swap is not implemented yet
// 	})

// 	It("should return orders for ws request", func() {
// 		wsURL := "ws://localhost:8080/ws/orders"
// 		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
// 		Expect(err).NotTo(HaveOccurred())
// 		defer conn.Close()
// 		subscribeMsg := []byte("subscribe:0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da")

// 		// Send the subscribe message to the server
// 		err = conn.WriteMessage(websocket.TextMessage, subscribeMsg)
// 		Expect(err).NotTo(HaveOccurred())

// 		// Receive the response from the server
// 		_, response, err := conn.ReadMessage()
// 		Expect(err).NotTo(HaveOccurred())
// 		fmt.Println("responseji: ", string(response))
// 		time.Sleep(5 * time.Second)
// 		var creatorOrderId uint
// 		creatorOrderId, err = c.CreateOrder("mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet:primary-ethereum_sepolia:0x4FDAAe676608f2a768f9c57BFDAeFA7559283316", "1", "10", "d87c01599e0f31a704ca73e5de993e274430101d4675d80da19d84b2bf19817d")
// 		count := 0
// 		for {
// 			_, message, err := conn.ReadMessage()
// 			Expect(err).NotTo(HaveOccurred())
// 			fmt.Println("message: ", string(message))
// 			count++
// 			if count >= 3 {
// 				break
// 			}
// 			time.Sleep(5 * time.Second)
// 			creatorOrderId, err = c.CreateOrder("mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet:primary-ethereum_sepolia:0x4FDAAe676608f2a768f9c57BFDAeFA7559283316", "1", "10", fmt.Sprintf("d87c01599e0f31a704ca73e5de993e274430101d4675d80da19d84b2bf19817%d", count))
// 			time.Sleep(5 * time.Second)
// 			c.FillOrder(creatorOrderId, "0xF403cE7776B22B74EcA871EcDaBeAA2103CD4A49", "mxHKgg7dU4pt9abWXveMofqRvWr7f6xx7g")
// 		}

// 	})

// })

// func StartServer() {
// 	go func() {
// 		store, err := store.New(sqlite.Open("gorm.db"), &gorm.Config{
// 			NowFunc: func() time.Time { return time.Now().UTC() },
// 		})
// 		Expect(err).NotTo(HaveOccurred())
// 		config := model.Config{
// 			Network: model.Network{
// 				model.BitcoinTestnet: model.NetworkConfig{
// 					Oracles: map[model.Asset]string{
// 						model.Primary: "https://api.coincap.io/v2/assets/bitcoin",
// 					},
// 					RPC:    "https://mempool.space/testnet/api",
// 					Expiry: 1000,
// 				},
// 				model.EthereumSepolia: model.NetworkConfig{
// 					Oracles: map[model.Asset]string{
// 						model.NewSecondary(""): "https://api.coincap.io/v2/assets/bitcoin",
// 					},
// 					RPC:    "http://localhost:8545",
// 					Expiry: 10000,
// 				},
// 			},
// 		}

// 		logger, err := zap.NewDevelopment()
// 		Expect(err).To(BeNil())
// 		s := rest.NewServer(store, config, logger, "PANTHER")
// 		s.Run(":8080")
// 	}()
// }
