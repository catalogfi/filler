package store

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/catalogfi/cobi/wbtc-garden/blockchain"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/rest"
	"github.com/catalogfi/cobi/wbtc-garden/watcher"
	"gorm.io/gorm"
)

type store struct {
	mu    *sync.RWMutex
	db    *gorm.DB
	cache map[string]blockchain.Price
}

type Store interface {
	rest.Store
	watcher.Store

	Gorm() *gorm.DB
}

func New(dialector gorm.Dialector, opts ...gorm.Option) (Store, error) {
	db, err := gorm.Open(dialector, opts...)
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&model.Order{}, &model.AtomicSwap{}, &model.Blacklist{}); err != nil {
		return nil, err
	}
	return &store{mu: new(sync.RWMutex), cache: make(map[string]blockchain.Price), db: db}, nil
}

// maintain a cache for the price and refresh it at the TTL interval
func (s *store) price(chain model.Chain, asset model.Asset, config model.Config) (blockchain.Price, error) {
	_, ok := config.Network[chain]
	if !ok {
		return blockchain.Price{}, fmt.Errorf("unsupported chain: %s", chain)
	}
	_, ok = config.Network[chain].Assets[asset]
	if !ok {
		return blockchain.Price{}, fmt.Errorf("unsupported asset: %s", asset)
	}

	priceObj, ok := s.cache[config.Network[chain].Assets[asset].Oracle]
	if !ok || time.Now().Unix()-priceObj.Timestamp > config.PriceTTL {
		updatedPrice, err := blockchain.GetPrice(config.Network[chain].Assets[asset].Oracle)
		if err != nil {
			return blockchain.Price{}, err
		}
		s.cache[config.Network[chain].Assets[asset].Oracle] = updatedPrice
		priceObj = updatedPrice
	}
	return priceObj, nil
}

// total amount of funds that are currently locked in active atomic swaps related to this system
func (s *store) ValueLockedByChain(chain model.Chain, config model.Network) (*big.Int, error) {
	// s.mu.RLock()
	// defer s.mu.RUnlock()
	swaps := []model.AtomicSwap{}
	if tx := s.db.Where("chain = ? AND status > ? AND status < ?", chain, model.NotStarted, model.Redeemed).Find(&swaps); tx.Error != nil {
		return nil, tx.Error
	}
	return s.usdValue(swaps, config)
}

// total amount of value traded by the user in the last 24 hrs in USD
func (s *store) ValueTradedByUserYesterday(user string, config model.Network) (*big.Int, error) {
	// s.mu.RLock()
	// defer s.mu.RUnlock()

	return s.valueTradedByUserYesterday(user, config)
}

func (s *store) valueTradedByUserYesterday(user string, config model.Network) (*big.Int, error) {
	yesterday := time.Now().Add(-24 * time.Hour).UTC()
	orders := []model.Order{}
	if tx := s.db.Where("(maker = ? OR taker = ?) AND status >= ? AND status < ? AND created_at >= ?", user, user, model.Created, model.FailedSoft, yesterday).Find(&orders); tx.Error != nil {
		return nil, tx.Error
	}
	if len(orders) == 0 {
		return big.NewInt(0), nil
	}
	swapIDs := make([]uint, len(orders))
	for i, order := range orders {
		swapIDs[i] = order.InitiatorAtomicSwapID
		if order.Taker == user {
			swapIDs[i] = order.FollowerAtomicSwapID
		}
	}
	swaps := []model.AtomicSwap{}
	if tx := s.db.Find(&swaps, swapIDs); tx.Error != nil {
		return nil, tx.Error
	}
	return s.usdValue(swaps, config)
}

// calculates the cummulative usd value of all the given swaps
func (s *store) usdValue(swaps []model.AtomicSwap, config model.Network) (*big.Int, error) {
	tvlF := big.NewFloat(0)

	// scoping the cache
	{
		cacheNormalisers := map[model.Asset]*big.Int{}
		for _, swap := range swaps {
			if swap.FilledAmount == "" {
				swap.FilledAmount = "0"
			}
			swapAmount, ok := new(big.Int).SetString(swap.Amount, 10)
			if !ok {
				return nil, fmt.Errorf("currupted value stored for amount: %v", swap.Amount)
			}
			normaliser, ok := cacheNormalisers[swap.Asset]
			if !ok {
				decimals := config[swap.Chain].Assets[swap.Asset].Decimals
				if decimals == 0 {
					return nil, fmt.Errorf("failed to get decimals for %v on %v", swap.Asset, swap.Chain)
				}
				normaliser = new(big.Int).Exp(big.NewInt(10), big.NewInt(decimals), nil)
				cacheNormalisers[swap.Asset] = normaliser
			}
			normalisedLockedAmount := new(big.Float).Quo(new(big.Float).SetInt(swapAmount), new(big.Float).SetInt(normaliser))
			lockedAmountValue := new(big.Float).Mul(big.NewFloat(swap.PriceByOracle), normalisedLockedAmount)
			tvlF.Add(lockedAmountValue, tvlF)

		}
	}

	// ignoring accuracy
	tvl, _ := tvlF.Int(nil)
	return tvl, nil
}

// create a new order with the given details
func (s *store) CreateOrder(creator, sendAddress, receiveAddress, orderPair, sendAmount, receiveAmount, secretHash string, userBtcWalletAddress string, config model.Config) (uint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// check if creatorAddress is valid eth address
	if err := blockchain.CheckAddress(model.Ethereum, creator); err != nil {
		return 0, err
	}
	sendChain, receiveChain, sendAsset, receiveAsset, err := model.ParseOrderPair(orderPair)
	if err != nil {
		return 0, err
	}
	_, ok := config.Network[sendChain]
	if !ok {
		return 0, fmt.Errorf("unsupported send chain")
	}
	_, ok = config.Network[receiveChain]
	if !ok {
		return 0, fmt.Errorf("unsupported recieve chain")
	}

	// check if send address and receive address are proper addresses for respective chains
	if err := blockchain.CheckAddress(receiveChain, receiveAddress); err != nil {
		return 0, fmt.Errorf("invalid recieve address: %v", err)
	}
	if err := blockchain.CheckAddress(sendChain, sendAddress); err != nil {
		return 0, fmt.Errorf("invalid send address: %v", err)
	}

	// TODO: can we make this more generic userBtcWalletAddress

	// validate secretHash
	if err := blockchain.CheckHash(secretHash); err != nil {
		return 0, err
	}
	sendAmt, ok := new(big.Int).SetString(sendAmount, 10)
	if !ok {
		return 0, fmt.Errorf("invalid send amount: %s", sendAmount)
	}

	receiveAmt, ok := new(big.Int).SetString(receiveAmount, 10)
	if !ok {
		return 0, fmt.Errorf("invalid receive amount: %s", receiveAmount)
	}

	if config.DailyLimit != "" {
		// get the total amount traded by the user in the last 24 hrs for limit checks
		tradedValue, err := s.ValueTradedByUserYesterday(creator, config.Network)
		if err != nil {
			return 0, err
		}
		dailyLimit, ok := new(big.Int).SetString(config.DailyLimit, 10)
		if !ok {
			return 0, fmt.Errorf("invalid daily limit: %v", err)
		}

		if tradedValue.Cmp(dailyLimit) >= 0 {
			return 0, fmt.Errorf("reached daily limit")
		}
	}

	initiatorSwapPrice, err := s.price(sendChain, sendAsset, config)
	if err != nil {
		return 0, fmt.Errorf("failed to get price for %s: %v", sendAsset, err)
	}

	followerSwapPrice, err := s.price(receiveChain, receiveAsset, config)
	if err != nil {
		return 0, fmt.Errorf("failed to get price for %s: %v", receiveAsset, err)
	}

	// get the number of orders to calculate user specific nonce
	orders, err := s.FilterOrders(creator, "", "", "", "", 0, 0, 0, 0, 0, 0, 0, false)
	if err != nil {
		return 0, err
	}

	// ignoring accuracy
	price, _ := new(big.Float).Quo(new(big.Float).SetInt(sendAmt), new(big.Float).SetInt(receiveAmt)).Float64()

	initiatorAtomicSwap := model.AtomicSwap{
		InitiatorAddress: sendAddress,
		Chain:            sendChain,
		Asset:            sendAsset,
		Amount:           sendAmount,
		PriceByOracle:    initiatorSwapPrice.Price,
	}

	sendValue, err := s.usdValue([]model.AtomicSwap{initiatorAtomicSwap}, config.Network)
	if err != nil {
		return 0, fmt.Errorf("invalid send value: %v", err)
	}

	if config.MinTxLimit != "" {
		// check if send amount is less than MinTxLimit
		minTxLimit, ok := new(big.Int).SetString(config.MinTxLimit, 10)
		if !ok {
			return 0, fmt.Errorf("invalid min limit: %v", err)
		}
		if sendValue.Cmp(minTxLimit) == -1 {
			return 0, fmt.Errorf("invalid send amount: %s", sendAmount)
		}
	}

	if config.MaxTxLimit != "" {
		// check if send amount is less than MinTxLimit
		maxTxLimit, ok := new(big.Int).SetString(config.MaxTxLimit, 10)
		if !ok {
			return 0, fmt.Errorf("invalid max limit: %v", err)
		}

		if sendValue.Cmp(maxTxLimit) == 1 {
			return 0, fmt.Errorf("invalid send amount: %s", sendAmount)
		}
	}

	followerAtomicSwap := model.AtomicSwap{
		RedeemerAddress: receiveAddress,
		Chain:           receiveChain,
		Asset:           receiveAsset,
		Amount:          receiveAmount,
		PriceByOracle:   followerSwapPrice.Price,
	}

	if tx := s.db.Create(&initiatorAtomicSwap); tx.Error != nil {
		return 0, tx.Error
	}
	if tx := s.db.Create(&followerAtomicSwap); tx.Error != nil {
		return 0, tx.Error
	}

	order := model.Order{
		Maker:                 creator,
		OrderPair:             orderPair,
		InitiatorAtomicSwapID: initiatorAtomicSwap.ID,
		FollowerAtomicSwapID:  followerAtomicSwap.ID,
		InitiatorAtomicSwap:   &initiatorAtomicSwap,
		FollowerAtomicSwap:    &followerAtomicSwap,
		Price:                 price,
		SecretHash:            secretHash,
		Status:                model.Created,
		SecretNonce:           uint64(len(orders)) + 1,
		UserBtcWalletAddress:  userBtcWalletAddress,
	}
	if tx := s.db.Create(&order); tx.Error != nil {
		return 0, tx.Error
	}

	return order.ID, nil
}

// fill an existing order by filling the required details for both the atomic swaps
func (s *store) FillOrder(orderID uint, filler, sendAddress, receiveAddress string, config model.Network) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	order := &model.Order{}
	if tx := s.db.First(order, orderID); tx.Error != nil {
		return tx.Error
	}
	if order.Status != model.Created {
		return fmt.Errorf("order already filled, current status: %v", order.Status)
	}
	fromChain, toChain, _, _, err := model.ParseOrderPair(order.OrderPair)
	if err != nil {
		return fmt.Errorf("constraint violation: corrupted order pair: %v", err)
	}
	if err := blockchain.CheckAddress(fromChain, receiveAddress); err != nil {
		return fmt.Errorf("invalid recieve address: %v", err)
	}
	if err := blockchain.CheckAddress(toChain, sendAddress); err != nil {
		return fmt.Errorf("invalid send address: %v", err)
	}
	initiateAtomicSwap := &model.AtomicSwap{}
	if tx := s.db.First(initiateAtomicSwap, order.InitiatorAtomicSwapID); tx.Error != nil {
		return tx.Error
	}
	followerAtomicSwap := &model.AtomicSwap{}
	if tx := s.db.First(followerAtomicSwap, order.FollowerAtomicSwapID); tx.Error != nil {
		return tx.Error
	}
	toChainAmount, err := s.ValueLockedByChain(toChain, config)
	if err != nil {
		return fmt.Errorf("failed to calculate value locked on %s: %v", toChain, err)
	}
	fromChainAmount, err := s.ValueLockedByChain(fromChain, config)
	if err != nil {
		return fmt.Errorf("failed to calculate value locked on %s: %v", toChain, err)
	}
	initiateAtomicSwap.RedeemerAddress = receiveAddress
	initiateAtomicSwap.Timelock = strconv.FormatInt(config[fromChain].Expiry*2, 10)
	initiateAtomicSwap.MinimumConfirmations = blockchain.GetMinConfirmations(fromChainAmount, fromChain)
	followerAtomicSwap.InitiatorAddress = sendAddress
	followerAtomicSwap.Timelock = strconv.FormatInt(config[toChain].Expiry, 10)
	followerAtomicSwap.MinimumConfirmations = blockchain.GetMinConfirmations(toChainAmount, toChain)
	order.Taker = filler
	order.Status = model.Filled
	if tx := s.db.Save(order); tx.Error != nil {
		return tx.Error
	}
	if tx := s.db.Save(initiateAtomicSwap); tx.Error != nil {
		return tx.Error
	}
	if tx := s.db.Save(followerAtomicSwap); tx.Error != nil {
		return tx.Error
	}
	return nil
}

// delete the given user's order if it is not filled
func (s *store) CancelOrder(creator string, orderID uint) error {
	order := &model.Order{}
	if tx := s.db.First(order, orderID); tx.Error != nil {
		return tx.Error
	}
	if order.Maker != creator {
		return fmt.Errorf("order can be cancelled only by its creator")
	}
	if order.Status != model.Created {
		return fmt.Errorf("order can be cancelled only if it is not filled, current status: %v", order.Status)
	}
	if tx := s.db.Delete(order); tx.Error != nil {
		return tx.Error
	}
	return nil
}

// filter the orders based on the given query parameters
func (s *store) FilterOrders(maker, taker, orderPair, secretHash, sort string, status model.Status, minPrice, maxPrice float64, minAmount, maxAmount float64, page, perPage int, verbose bool) ([]model.Order, error) {
	orders := []model.Order{}
	tx := s.db.Table("orders")
	if orderPair != "" {
		tx = tx.Where("order_pair = ?", orderPair)
	}
	joinAtomicSwaps := false
	if minAmount != 0 {
		joinAtomicSwaps = true
		tx = tx.Where("atomic_swaps.amount >= ?", uint(minAmount))
	}
	if maxAmount != 0 {
		joinAtomicSwaps = true
		tx = tx.Where("atomic_swaps.amount <= ?", uint(maxAmount))
	}
	if joinAtomicSwaps {
		tx = tx.Joins("JOIN atomic_swaps ON orders.initiator_atomic_swap_id = atomic_swaps.id ")
	}
	if minPrice != 0 {
		tx = tx.Where("price >= ?", minPrice)
	}
	if maxPrice != 0 {
		tx = tx.Where("price <= ?", maxPrice)
	}
	if status != model.Unknown {
		tx = tx.Where("orders.status = ?", status)
	}
	if maker != "" {
		tx = tx.Where("maker = ?", maker)
	}
	if taker != "" {
		tx = tx.Where("taker = ?", taker)
	}
	if secretHash != "" {
		tx = tx.Where("orders.secret_hash = ?", secretHash)
	}

	// sort
	orderByList := strings.Split(sort, ",")
	orderByQuery := ""
	for _, orderBy := range orderByList {
		if orderBy == "" {
			continue
		}
		if orderByQuery != "" {
			orderByQuery += ", "
		}
		if orderBy[0] == '-' {
			orderByQuery += orderBy[1:] + " DESC"
		} else {
			orderByQuery += orderBy + ""
		}
	}
	if orderByQuery != "" {
		tx = tx.Order(orderByQuery)
	}

	// pagination
	if page != 0 && perPage != 0 {
		tx = tx.Offset((page - 1) * perPage).Limit(perPage)
	}
	if tx = tx.Find(&orders); tx.Error != nil {
		return nil, tx.Error
	}

	if verbose {
		for i := range orders {
			if err := s.fillSwapDetails(&orders[i]); err != nil {
				return nil, err
			}
		}
	}
	return orders, nil
}

// filter the orders based on the given query parameters
func (s *store) GetOrdersByAddress(address string) ([]model.Order, error) {
	orders := []model.Order{}
	if tx := s.db.Where("maker = ? OR taker = ?", address, address).Find(&orders); tx.Error != nil {
		return nil, tx.Error
	}
	for i := range orders {
		if err := s.fillSwapDetails(&orders[i]); err != nil {
			return nil, err
		}
	}
	return orders, nil
}

// get all the orders with active atomic swaps
func (s *store) GetActiveOrders() ([]model.Order, error) {
	orders := []model.Order{}
	if tx := s.db.Where("status IN ?", []model.Status{model.Created, model.Filled}).Find(&orders); tx.Error != nil {
		return nil, tx.Error
	}
	for i := range orders {
		if err := s.fillSwapDetails(&orders[i]); err != nil {
			return nil, err
		}
	}
	return orders, nil
}

// get all the swaps that are active
func (s *store) GetActiveSwaps(chain model.Chain) ([]model.AtomicSwap, error) {
	swaps := []model.AtomicSwap{}
	if tx := s.db.Where("status IN ? AND chain = ?", []model.SwapStatus{model.NotStarted, model.Initiated, model.Detected, model.Expired}, chain).Find(&swaps); tx.Error != nil {
		return nil, tx.Error
	}
	return swaps, nil
}

func (s *store) SwapByOCID(ocID string) (model.AtomicSwap, error) {
	swap := model.AtomicSwap{}
	if tx := s.db.Where("on_chain_identifier = ?", ocID).First(&swap); tx.Error != nil {
		return model.AtomicSwap{}, tx.Error
	}
	return swap, nil
}

// update the given atomic swap objects on the db
// @dev should only be used internally and cannot be called by an end user
func (s *store) UpdateSwap(swap *model.AtomicSwap) error {
	if tx := s.db.Save(swap); tx.Error != nil {
		return tx.Error
	}
	return nil
}

// get the order with the given order id
func (s *store) GetOrder(orderID uint) (*model.Order, error) {
	order := &model.Order{
		InitiatorAtomicSwap: &model.AtomicSwap{},
		FollowerAtomicSwap:  &model.AtomicSwap{},
	}
	if tx := s.db.First(order, orderID); tx.Error != nil {
		return nil, tx.Error
	}
	return order, s.fillSwapDetails(order)
}

// update the given order and the internal atomic swap objects on the db
// @dev should only be used internally and cannot be called by an end user
func (s *store) UpdateOrder(order *model.Order) error {
	if tx := s.db.Save(order.FollowerAtomicSwap); tx.Error != nil {
		return tx.Error
	}
	if tx := s.db.Save(order.InitiatorAtomicSwap); tx.Error != nil {
		return tx.Error
	}
	if tx := s.db.Save(order); tx.Error != nil {
		return tx.Error
	}
	return nil
}

// fills the atomic swap objects in the given order
func (s *store) fillSwapDetails(order *model.Order) error {
	order.FollowerAtomicSwap = &model.AtomicSwap{}
	order.InitiatorAtomicSwap = &model.AtomicSwap{}
	if tx := s.db.First(order.InitiatorAtomicSwap, order.InitiatorAtomicSwapID); tx.Error != nil {
		return tx.Error
	}
	if tx := s.db.First(order.FollowerAtomicSwap, order.FollowerAtomicSwapID); tx.Error != nil {
		return tx.Error
	}
	return nil
}

func (s *store) Gorm() *gorm.DB {
	return s.db
}
